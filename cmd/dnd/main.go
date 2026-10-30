// Command dnd 是 Dungeons & Diffs 的 CLI，只負責組裝：解析參數 → 建 Event → store.Update。
// 遊戲判斷全部在 internal/engine。debug 子指令只在 DND_DEBUG=1 時註冊。
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
		Use:           "dnd",
		Short:         "Dungeons & Diffs：用 commit 與 AI 工作養寵物、闖地下城",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newEventCmd())
	if os.Getenv("DND_DEBUG") == "1" {
		root.AddCommand(newDebugCmd())
	}
	return root
}

func openStore() (*store.FileStore, error) {
	dir, err := home.Dir()
	if err != nil {
		return nil, err
	}
	pack, err := content.Default()
	if err != nil {
		return nil, err
	}
	return store.Open(dir, pack)
}

func printf(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.OutOrStdout(), format, a...)
}
