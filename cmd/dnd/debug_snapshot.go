package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/store"
	"github.com/spf13/cobra"
)

// `dnd debug snapshot save|load|list`：把 DND_HOME 的遊戲資料存到 $DND_HOME/snapshots/<名稱>/，
// 之後可以反覆載回同一個狀態測試。存與載都在 store 的寫入鎖內進行，不會和 hook 同時寫。
// load 前先把目前的資料另存成 before-load-<時間>，載錯了還能回去。
//
// 只複製 snapshotFiles；lock、capture／、claude／（安裝 Claude Code 時的設定）不在快照裡。
// 快照裡沒有的檔案（例如沒有時間偏移）在 load 時會從 DND_HOME 刪掉，讓狀態完全一致。

const snapshotDir = "snapshots"

var snapshotFiles = []string{"events.jsonl", "state.json", clock.OffsetFile, "errors.log"}

// snapshotLockTimeout 比 hook 的 50 ms 寬鬆：除錯指令可以等 hook 寫完。
const snapshotLockTimeout = 2 * time.Second

func newDebugSnapshotCmd() *cobra.Command {
	snap := &cobra.Command{Use: "snapshot", Short: "存取整個 DND_HOME，在同一個狀態反覆測試"}
	var force bool
	save := &cobra.Command{
		Use:   "save <名稱>",
		Short: "把目前的資料存成快照",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withSnapshots(args[0], func(s *store.FileStore, dir, dst string) error {
				if _, err := os.Stat(dst); err == nil && !force {
					return fmt.Errorf("快照 %q 已存在（--force 覆寫）", args[0])
				}
				if err := s.WithLock(func() error { return copySnapshot(dir, dst) }); err != nil {
					return err
				}
				printf(cmd, "已存成快照 %s（%s）\n", args[0], describeSnapshot(dst))
				return nil
			})
		},
	}
	save.Flags().BoolVar(&force, "force", false, "覆寫同名快照")

	load := &cobra.Command{
		Use:   "load <名稱>",
		Short: "載回快照（先自動另存目前的資料為 before-load-時間）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withSnapshots(args[0], func(s *store.FileStore, dir, src string) error {
				if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
					return fmt.Errorf("沒有快照 %q（dnd debug snapshot list）", args[0])
				}
				base := filepath.Join(dir, snapshotDir, "before-load-"+time.Now().Format("20060102-150405"))
				backup := base
				for i := 2; exists(backup); i++ {
					backup = fmt.Sprintf("%s-%d", base, i)
				}
				err := s.WithLock(func() error {
					if err := copySnapshot(dir, backup); err != nil {
						return fmt.Errorf("另存目前的資料失敗，沒有載入：%w", err)
					}
					return copySnapshot(src, dir)
				})
				if err != nil {
					return err
				}
				printf(cmd, "已載入快照 %s（%s）；原本的資料另存為 %s\n", args[0], describeSnapshot(src), filepath.Base(backup))
				return nil
			})
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "列出快照",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := home.Dir()
			if err != nil {
				return err
			}
			entries, err := os.ReadDir(filepath.Join(dir, snapshotDir))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			n := 0
			for _, e := range entries {
				if e.IsDir() {
					printf(cmd, "%s  %s\n", e.Name(), describeSnapshot(filepath.Join(dir, snapshotDir, e.Name())))
					n++
				}
			}
			if n == 0 {
				printf(cmd, "還沒有快照（dnd debug snapshot save <名稱>）\n")
			}
			return nil
		},
	}
	snap.AddCommand(save, load, list)
	return snap
}

// withSnapshots 檢查快照名稱、開 store，把 DND_HOME 與快照目錄交給 f。
func withSnapshots(name string, f func(s *store.FileStore, dir, path string) error) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("快照名稱不能是空的、. 或 ..，也不能含路徑分隔符號：%q", name)
	}
	dir, err := home.Dir()
	if err != nil {
		return err
	}
	s, _, err := openStore()
	if err != nil {
		return err
	}
	s.LockTimeout = snapshotLockTimeout
	return f(s, dir, filepath.Join(dir, snapshotDir, name))
}

// copySnapshot 把 src 的 snapshotFiles 複製到 dst（不存在就建立），src 沒有的檔案在 dst 刪掉。
// 每個檔案先寫暫存檔再 rename，中途失敗不會留下半個檔案。
func copySnapshot(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, name := range snapshotFiles {
		from, to := filepath.Join(src, name), filepath.Join(dst, name)
		data, err := os.ReadFile(from)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(to); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := writeFileAtomic(to, data); err != nil {
			return err
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// describeSnapshot 回傳「N 個事件，存於 時間」（時間是快照目錄最後修改的真實時間，不含偏移）。
func describeSnapshot(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	desc := fmt.Sprintf("%d 個事件", bytes.Count(b, []byte("\n")))
	if fi, err := os.Stat(dir); err == nil {
		desc += "，存於 " + fi.ModTime().Format("2006-01-02 15:04")
	}
	return desc
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
