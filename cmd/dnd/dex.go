package main

import (
	"time"

	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// `dnd dex`：圖鑑，列出所有怪物（收服過、遇過、沒遇過的顯示 ???）。讀法同 status：不取鎖、不寫檔。
func newDexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dex",
		Short: "圖鑑：遇過與收服過的怪物",
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
			printf(cmd, "%s", view.Dex(st, pack))
			return nil
		},
	}
}

// `dnd party`：顯示隊伍；`dnd party add|remove <怪物 id>` 寫入 party 事件調整隊伍。
// CLI 只檢查參數個數；能不能加入或移除由引擎判斷，不合法時引擎回一句日誌、狀態不變。
func newPartyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "party",
		Short: "隊伍：收服的怪物讓寵物更強，也加強合體技",
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
			now := time.Now()
			st = engine.Apply(st, event.Event{At: now, Source: event.SourceTick, Type: "tick"}, pack).State
			printf(cmd, "%s", view.Party(st, pack, now))
			return nil
		},
	}
	for _, op := range []struct{ use, short string }{
		{"add", "讓收服過的怪物加入隊伍（最多 3 隻）"},
		{"remove", "讓隊員離開隊伍"},
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   op.use + " <怪物 id>",
			Short: op.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s, _, err := openStore()
				if err != nil {
					return err
				}
				r, err := s.Update(event.Event{
					At: time.Now(), Source: event.SourceUser, Type: "party",
					Args: map[string]string{"op": op.use, "id": args[0]},
				})
				if err != nil {
					return err
				}
				for _, l := range r.Logs {
					printf(cmd, "%s\n", l.Text)
				}
				return nil
			},
		})
	}
	return cmd
}
