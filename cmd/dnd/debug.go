package main

import "github.com/spf13/cobra"

// debug 子指令只在 DND_DEBUG=1 時註冊，不會混進正式玩法（DEVPLAN「自我測試與迭代流程」）：
// rebuild、time（debug_time.go）、xp／floor（debug_game.go）、event（與 `dnd event` 同一個指令，event.go）、
// snapshot（debug_snapshot.go）、stats（debug_stats.go）。開發測試一律搭配 DND_HOME=./tmp/dnd。
func newDebugCmd() *cobra.Command {
	debug := &cobra.Command{Use: "debug", Short: "除錯指令（DND_DEBUG=1）"}
	debug.AddCommand(&cobra.Command{
		Use:   "rebuild",
		Short: "從 events.jsonl 重播並覆寫 state.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, _, err := openStore()
			if err != nil {
				return err
			}
			st, err := s.Rebuild()
			if err != nil {
				return err
			}
			printf(cmd, "重建完成：%d 個事件，content %s\n", st.LastSeq, st.ContentHash)
			return nil
		},
	})
	debug.AddCommand(newDebugTimeCmd(), newDebugXPCmd(), newDebugFloorCmd(), newEventCmd(), newDebugSnapshotCmd(), newDebugStatsCmd())
	return debug
}
