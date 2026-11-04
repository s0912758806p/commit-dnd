package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd warp <樓層>`：寫入 warp 事件，傳送到入口（0）或已解鎖的 Boss 層。
// CLI 只檢查參數是非負整數；能不能傳送由引擎判斷，不合法時引擎回一句日誌、狀態不變。
func newWarpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "warp <樓層>",
		Short: "傳送到入口（0）或打倒過 Boss 的樓層",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			floor, err := strconv.Atoi(args[0])
			if err != nil || floor < 0 {
				return fmt.Errorf("樓層要是非負整數（0 是入口）：%q", args[0])
			}
			s, _, err := openStore()
			if err != nil {
				return err
			}
			r, err := s.Update(event.Event{
				At: time.Now(), Source: event.SourceUser, Type: "warp",
				Args: map[string]string{"floor": strconv.Itoa(floor)},
			})
			if err != nil {
				return err
			}
			for _, l := range r.Logs {
				printf(cmd, "%s\n", l.Text)
			}
			return nil
		},
	}
}
