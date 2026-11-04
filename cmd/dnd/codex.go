package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	codexadapter "github.com/s0912758806p/commit-dnd/internal/adapter/codex"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/install"
	"github.com/spf13/cobra"
)

// Codex CLI 整合（迭代 4）：`dnd hook codex`、`dnd install／uninstall codex`、doctor 的 Codex 檢查。

// newHookCodexCmd 是 Codex 的 notify：`dnd hook codex <payload JSON>`。
// Codex 把 payload 附在最後一個參數、stdin 是空的，也不等 notify 結束、不看 exit code。
// 先寫 dnd 自己的事件，再用同樣的參數執行安裝前原本的 notify（有串接時）；兩邊各自吞掉錯誤，互不影響。
func newHookCodexCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "codex",
		Short:              "Codex 的 notify：把 AI 回合寫成事件",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true, // payload 與原本 notify 的參數原樣保留
		Run: func(_ *cobra.Command, args []string) {
			runHook("codex", func() error { return hookCodex(args) })
			runHook("codex notify 串接", func() error { return chainCodexNotify(args) })
		},
	}
}

func hookCodex(args []string) error {
	if len(args) == 0 {
		return errors.New("沒有收到 notify payload（Codex 會放在最後一個參數）")
	}
	raw := []byte(args[len(args)-1])
	captureCodex(raw)
	events, err := codexadapter.Adapter{}.Parse(raw, time.Now())
	if err != nil || len(events) == 0 {
		return err
	}
	s, _, err := openStore()
	if err != nil {
		return err
	}
	for _, e := range events {
		if _, err := s.Update(e); err != nil {
			return err
		}
	}
	return nil
}

// chainCodexNotify 用同樣的參數執行被 dnd 取代前的 notify；跟 Codex 一樣只啟動、不等它結束。
func chainCodexNotify(args []string) error {
	dir, err := home.Dir()
	if err != nil {
		return err
	}
	orig, err := install.ReadOriginalNotify(dir)
	if err != nil || len(orig) == 0 || install.IsDndNotify(orig) {
		return err
	}
	c := exec.Command(orig[0], append(orig[1:], args...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// captureCodex 在 $DND_HOME/capture/codex 存在時，把原始 payload 依 type 存一份（同類覆寫），
// 用來收集 testdata 樣本。平常這個目錄不存在，什麼都不做。
func captureCodex(raw []byte) {
	dir, err := home.Dir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "capture", "codex")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return
	}
	var p struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &p)
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '.' {
			return '_'
		}
		return r
	}, p.Type)
	if name == "" {
		name = "unknown"
	}
	_ = os.WriteFile(filepath.Join(dir, name+".json"), raw, 0o600)
}

// newCodexCmd 建立 `install codex`（install 為 true）或 `uninstall codex`。
func newCodexCmd(installing bool) *cobra.Command {
	var config string
	var yes bool
	short := "在 Codex 的 config.toml 設定 notify"
	if !installing {
		short = "從 Codex 的 config.toml 移除 dnd 的 notify，還原原本的設定"
	}
	c := &cobra.Command{
		Use:   "codex",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := install.CodexConfigPath(config)
			if err != nil {
				return err
			}
			dndHome, err := home.Dir()
			if err != nil {
				return err
			}
			var plan *install.ConfigPlan
			var self string
			if installing {
				if self, err = selfPath(); err != nil {
					return err
				}
				plan, err = install.PlanCodexInstall(path, self, dndHome)
			} else {
				plan, err = install.PlanCodexUninstall(path, dndHome)
			}
			if err != nil {
				return err
			}
			if !plan.Changed() {
				if installing {
					printf(cmd, "已經安裝過 %s\n", path)
				} else {
					printf(cmd, "沒有安裝（%s 的 notify 不是 dnd）\n", path)
				}
				return nil
			}
			ok, err := plan.Apply(install.Confirm{
				In: cmd.InOrStdin(), Out: cmd.OutOrStdout(),
				Interactive: interactive(cmd), Yes: yes, Now: time.Now(),
			})
			if err != nil || !ok {
				return err
			}
			if installing {
				printf(cmd, "已寫入 %s\n之後每個 Codex 回合結束都會呼叫 %s（已經開著的 Codex 要重新啟動才會讀到）\n", path, self)
			} else {
				printf(cmd, "已從 %s 移除 dnd\n", path)
			}
			return nil
		},
	}
	c.Flags().StringVar(&config, "config", "", "config.toml 路徑（預設 $CODEX_HOME/config.toml 或 ~/.codex/config.toml）")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "不詢問直接寫入")
	return c
}

// codex 檢查 Codex 的 notify。不是每個人都用 Codex，所以沒有設定檔或沒有接上時只提醒（!）。
func (d *doctor) codex(config string) {
	dir, err := home.Dir()
	if err != nil {
		d.fail("Codex", "%v", err)
		return
	}
	st, err := install.CodexConfigStatus(config, dir)
	switch {
	case err != nil:
		d.fail("Codex", "讀不了 %s：%v", config, err)
	case !st.Exists:
		d.warn("Codex", "沒有 %s；不用 Codex 可以忽略，要接上 → dnd install codex", config)
	case !st.Installed:
		d.warn("Codex", "notify 沒有接上 dnd（%s）；不用 Codex 可以忽略，要接上 → dnd install codex", config)
	case !executable(st.DndPath):
		d.fail("Codex", "notify 指向的 %s 不存在或不能執行 → 重新 dnd install codex", st.DndPath)
	default:
		d.ok("Codex", "notify 已安裝（%s）", config)
		if len(st.Original) > 0 {
			d.ok("Codex notify 串接", "會再執行安裝前原本的 notify：%s", strings.Join(st.Original, " "))
		}
	}
}
