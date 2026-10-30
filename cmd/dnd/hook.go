package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	gitadapter "github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/spf13/cobra"
)

// `dnd hook git`：由 post-commit 呼叫（隱藏指令）。不輸出任何東西、永遠 exit 0。
func newHookCmd() *cobra.Command {
	hook := &cobra.Command{Use: "hook", Short: "給外部工具呼叫的 hook", Hidden: true}
	hook.AddCommand(&cobra.Command{
		Use:   "git",
		Short: "post-commit：把剛完成的 commit 寫成事件",
		Args:  cobra.ArbitraryArgs,
		Run:   func(*cobra.Command, []string) { runHook("git", hookGit) },
	})
	return hook
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
	events, err := gitadapter.Adapter{Repo: filepath.Base(wd)}.Parse(out, time.Now())
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
