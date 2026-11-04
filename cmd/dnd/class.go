package main

import (
	"time"

	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// `dnd class`：顯示職業、特化、進化與 commit 組成（讀法同 status，不寫檔）。
// `dnd class choose <特化 id>`、`dnd class reroll`：寫入 class 事件；能不能選、能不能用卷軸由引擎判斷，
// 不合法時引擎回一句日誌、狀態不變。
func newClassCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "class",
		Short: "職業、特化與進化",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, pack, err := openStore()
			if err != nil {
				return err
			}
			st, err := s.Load()
			if err != nil {
				return err
			}
			st = engine.Apply(st, event.Event{At: time.Now(), Source: event.SourceTick, Type: "tick"}, pack).State
			printf(cmd, "%s", view.Class(st, pack))
			return nil
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "choose <特化 id>",
		Short: "選擇特化（Lv 15 起，一次；要換請用轉職卷軸）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return classEvent(cmd, map[string]string{"action": "choose", "spec": args[0]})
		},
	}, &cobra.Command{
		Use:   "reroll",
		Short: "使用轉職卷軸，依目前的 commit 組成重新判定職業（等級不變、特化重選）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return classEvent(cmd, map[string]string{"action": "reroll"})
		},
	})
	return cmd
}

func classEvent(cmd *cobra.Command, args map[string]string) error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	r, err := s.Update(event.Event{At: time.Now(), Source: event.SourceUser, Type: "class", Args: args})
	if err != nil {
		return err
	}
	for _, l := range r.Logs {
		printf(cmd, "%s\n", l.Text)
	}
	return nil
}
