// Command dnd 是 Dungeons & Diffs 的 CLI，只負責組裝：解析參數 → 建 Event → store.Update，
// 或讀 State → view 排版。遊戲判斷全部在 internal/engine。
//
// hook 子指令由 git 等外部工具呼叫，任何錯誤都寫進 $DND_HOME/errors.log 後以 exit 0 結束，
// 不能卡住使用者的 commit。debug 子指令只在 DND_DEBUG=1 時註冊。
package main

import (
	"fmt"
	"os"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/store"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:          "dnd",
		Short:        "Dungeons & Diffs：用 commit 與 AI 工作養寵物、闖地下城",
		SilenceUsage: true,
	}
	root.AddCommand(newHatchCmd(), newStatusCmd(), newLogCmd(), newInstallCmd(), newUninstallCmd(), newHookCmd(), newEventCmd())
	if os.Getenv("DND_DEBUG") == "1" {
		root.AddCommand(newDebugCmd())
	}
	return root
}

func openStore() (*store.FileStore, *content.Pack, error) {
	dir, err := home.Dir()
	if err != nil {
		return nil, nil, err
	}
	pack, err := content.Default()
	if err != nil {
		return nil, nil, err
	}
	s, err := store.Open(dir, pack)
	return s, pack, err
}

func printf(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.OutOrStdout(), format, a...)
}
