package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/install"
	"github.com/spf13/cobra"
)

// `dnd install git [--repo dir]` / `dnd uninstall git`：在 repo 的 post-commit 加入／移除 dnd 區塊。
// `dnd install claude` / `dnd uninstall claude [--settings path] [--yes]`：編輯 Claude Code 的
// settings.json（hooks 與 statusLine），寫入前印 diff、確認並備份，細節見 internal/install/claude.go。
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
	parent.AddCommand(git, newClaudeCmd(true))
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
	parent.AddCommand(git, newClaudeCmd(false))
	return parent
}

// newClaudeCmd 建立 `install claude`（install 為 true）或 `uninstall claude`。
func newClaudeCmd(installing bool) *cobra.Command {
	var settings string
	var yes bool
	short := "在 Claude Code 的 settings.json 加入 hook 與 statusLine"
	if !installing {
		short = "從 Claude Code 的 settings.json 移除 dnd 的 hook 與 statusLine"
	}
	c := &cobra.Command{
		Use:   "claude",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := install.ClaudeSettingsPath(settings)
			if err != nil {
				return err
			}
			dndHome, err := home.Dir()
			if err != nil {
				return err
			}
			var plan *install.ClaudePlan
			var self string
			if installing {
				if self, err = selfPath(); err != nil {
					return err
				}
				plan, err = install.PlanClaudeInstall(path, self, dndHome)
			} else {
				plan, err = install.PlanClaudeUninstall(path, dndHome)
			}
			if err != nil {
				return err
			}
			if !plan.Changed() {
				if installing {
					printf(cmd, "已經安裝過 %s\n", path)
				} else {
					printf(cmd, "沒有安裝（%s 裡沒有 dnd）\n", path)
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
				printf(cmd, "已寫入 %s\nClaude Code 會自動重新載入設定，之後的工具呼叫與回合結束都會呼叫 %s\n", path, self)
			} else {
				printf(cmd, "已從 %s 移除 dnd\n", path)
			}
			return nil
		},
	}
	c.Flags().StringVar(&settings, "settings", "", "settings.json 路徑（預設 $CLAUDE_CONFIG_DIR/settings.json 或 ~/.claude/settings.json）")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "不詢問直接寫入")
	return c
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
