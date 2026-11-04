package main

import (
	"time"

	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// `dnd dungeon`：地下城的進度、眼前的怪物與裝備。讀法同 status：在記憶體套用 tick，不取鎖、不寫檔。
func newDungeonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dungeon",
		Short: "看看地下城的進度與裝備",
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
			printf(cmd, "%s", view.Dungeon(st, pack))
			return nil
		},
	}
}
