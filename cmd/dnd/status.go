package main

import (
	"time"

	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// `dnd status`：讀 state.json，在記憶體套用「現在」的 tick 後顯示。不取鎖、不寫檔。
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "看看寵物現在的狀態",
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
			printf(cmd, "%s", view.Status(st, pack, now))
			return nil
		},
	}
}

// `dnd log [-n N]`：從 events.jsonl 重播出冒險日誌，顯示最近 N 則。
func newLogCmd() *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "冒險日誌",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, _, err := openStore()
			if err != nil {
				return err
			}
			logs, err := s.History()
			if err != nil {
				return err
			}
			if n > 0 && len(logs) > n {
				logs = logs[len(logs)-n:]
			}
			printf(cmd, "%s", view.Log(logs))
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "number", "n", 20, "顯示最近幾則（0 表示全部）")
	return cmd
}
