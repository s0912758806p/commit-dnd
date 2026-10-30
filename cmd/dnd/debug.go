package main

import "github.com/spf13/cobra"

// debug 子指令只在 DND_DEBUG=1 時註冊，不會混進正式玩法。
func newDebugCmd() *cobra.Command {
	debug := &cobra.Command{Use: "debug", Short: "除錯指令（DND_DEBUG=1）"}
	debug.AddCommand(&cobra.Command{
		Use:   "rebuild",
		Short: "從 events.jsonl 重播並覆寫 state.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
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
	return debug
}
