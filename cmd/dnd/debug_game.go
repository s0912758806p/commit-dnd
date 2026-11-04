package main

import (
	"fmt"
	"strconv"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd debug xp <N>`、`dnd debug floor <N>`：寫入 debug_xp／debug_floor 事件，規則在 internal/engine/debug.go。
// 事件會留在 events.jsonl，重播結果一致。
func newDebugXPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "xp <N>",
		Short: "直接給 N 點 XP（照正常流程升級）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				return fmt.Errorf("XP 要是正整數：%q", args[0])
			}
			return writeDebugEvent(cmd, "debug_xp", "xp", n)
		},
	}
}

func newDebugFloorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "floor <N>",
		Short: "跳到第 N 層（0 是入口；清掉眼前的怪物、這層進度歸零）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil || n < 0 {
				return fmt.Errorf("樓層要是非負整數（0 是入口）：%q", args[0])
			}
			return writeDebugEvent(cmd, "debug_floor", "floor", n)
		},
	}
}

func writeDebugEvent(cmd *cobra.Command, typ, key string, n int) error {
	s, _, err := openStore()
	if err != nil {
		return err
	}
	r, err := s.Update(event.Event{
		At: clock.Now(), Source: event.SourceUser, Type: typ,
		Args: map[string]string{key: strconv.Itoa(n)},
	})
	if err != nil {
		return err
	}
	for _, l := range r.Logs {
		printf(cmd, "%s\n", l.Text)
	}
	return nil
}
