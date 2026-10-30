package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd event <source> <type> [key=value...]`：手動寫入一個事件，開發與驗收用。
// 不是 hook，錯誤照常回報。
func newEventCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "event <source> <type> [key=value...]",
		Short: "手動寫入一個事件（開發用）",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			e := event.Event{At: time.Now(), Source: event.Source(args[0]), Type: args[1]}
			if !e.Source.Valid() {
				return fmt.Errorf("未知來源 %q（git|claude|codex|user|tick）", args[0])
			}
			for _, kv := range args[2:] {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("參數要寫成 key=value：%q", kv)
				}
				if e.Args == nil {
					e.Args = map[string]string{}
				}
				e.Args[k] = v
			}

			s, err := openStore()
			if err != nil {
				return err
			}
			r, err := s.Update(e)
			if err != nil {
				return err
			}
			printf(cmd, "#%d %s/%s\n", r.State.LastSeq, e.Source, e.Type)
			for _, l := range r.Logs {
				printf(cmd, "  %s\n", l.Text)
			}
			return nil
		},
	}
}
