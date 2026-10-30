package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/s0912758806p/commit-dnd/internal/install"
	"github.com/spf13/cobra"
)

// `dnd install git [--repo dir]` / `dnd uninstall git`：在 repo 的 post-commit 加入／移除 dnd 區塊。
func newInstallCmd() *cobra.Command {
	var repo string
	parent := &cobra.Command{Use: "install", Short: "把 dnd 接進外部工具"}
	git := &cobra.Command{
		Use:   "git",
		Short: "在 repo 安裝 post-commit hook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			self, err := selfPath()
			if err != nil {
				return err
			}
			dir, err := install.GitHooksDir(repo)
			if err != nil {
				return err
			}
			msg, err := install.InstallGitHook(dir, self)
			if err != nil {
				return err
			}
			printf(cmd, "%s\n之後每次 commit 都會呼叫 %s\n", msg, self)
			return nil
		},
	}
	git.Flags().StringVar(&repo, "repo", ".", "repo 路徑")
	parent.AddCommand(git)
	return parent
}

func newUninstallCmd() *cobra.Command {
	var repo string
	parent := &cobra.Command{Use: "uninstall", Short: "移除 dnd 的外部整合"}
	git := &cobra.Command{
		Use:   "git",
		Short: "從 repo 移除 post-commit hook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := install.GitHooksDir(repo)
			if err != nil {
				return err
			}
			msg, err := install.UninstallGitHook(dir)
			if err != nil {
				return err
			}
			printf(cmd, "%s\n", msg)
			return nil
		},
	}
	git.Flags().StringVar(&repo, "repo", ".", "repo 路徑")
	parent.AddCommand(git)
	return parent
}

// selfPath 回傳目前執行檔的絕對路徑；go run 的暫存執行檔不能拿來當 hook。
func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err = filepath.EvalSymlinks(p); err != nil {
		return "", err
	}
	if strings.Contains(p, string(filepath.Separator)+"go-build") {
		return "", fmt.Errorf("目前是 go run 的暫存執行檔，請先 make install 再用安裝好的 dnd 執行")
	}
	return p, nil
}
