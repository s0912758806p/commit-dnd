package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	claudeadapter "github.com/s0912758806p/commit-dnd/internal/adapter/claude"
	gitadapter "github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/spf13/cobra"
)

// `dnd hook git`（post-commit）與 `dnd hook claude`（Claude Code hook）：隱藏指令，不輸出任何東西、永遠 exit 0。
func newHookCmd() *cobra.Command {
	hook := &cobra.Command{Use: "hook", Short: "給外部工具呼叫的 hook", Hidden: true}
	hook.AddCommand(&cobra.Command{
		Use:   "git",
		Short: "post-commit：把剛完成的 commit 寫成事件",
		Args:  cobra.ArbitraryArgs,
		Run:   func(*cobra.Command, []string) { runHook("git", hookGit) },
	})
	hook.AddCommand(&cobra.Command{
		Use:   "claude",
		Short: "Claude Code 的 PostToolUse／Stop：把 AI 活動寫成事件",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			runHook("claude", func() error { return hookClaude(cmd.InOrStdin()) })
		},
	})
	return hook
}

func hookClaude(stdin io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(stdin, 64<<20))
	if err != nil {
		return err
	}
	captureClaude(raw)
	events, err := claudeadapter.Adapter{}.Parse(raw, clock.Now())
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

// captureClaude 在 $DND_HOME/capture/claude 存在時，把原始 hook JSON 依事件與工具名稱存一份
// （同類覆寫），用來收集 testdata 樣本。平常這個目錄不存在，什麼都不做。
func captureClaude(raw []byte) {
	dir, err := home.Dir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "capture", "claude")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return
	}
	var p struct {
		Event string `json:"hook_event_name"`
		Tool  string `json:"tool_name"`
	}
	_ = json.Unmarshal(raw, &p)
	name := strings.Trim(p.Event+"-"+p.Tool, "-")
	if name == "" {
		name = "unknown"
	}
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, name)
	_ = os.WriteFile(filepath.Join(dir, name+".json"), raw, 0o600)
}

func hookGit() error {
	out, err := exec.Command("git", gitadapter.Command...).Output()
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	wd, err := os.Getwd() // git 在 worktree 根目錄執行 hook
	if err != nil {
		return err
	}
	events, err := gitadapter.Adapter{Repo: filepath.Base(wd)}.Parse(out, clock.Now())
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

// runHook 執行 hook；錯誤與 panic 都寫進 $DND_HOME/errors.log 後吞掉。
func runHook(name string, f func() error) {
	defer func() {
		if p := recover(); p != nil {
			logHookError(name, fmt.Errorf("panic: %v", p))
		}
	}()
	if err := f(); err != nil {
		logHookError(name, err)
	}
}

func logHookError(name string, err error) {
	dir, derr := home.Dir()
	if derr != nil || os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, ferr := os.OpenFile(filepath.Join(dir, "errors.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [hook %s] %v\n", time.Now().Format(time.RFC3339), name, err)
}
