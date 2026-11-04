package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd rebirth [--yes]`：取得終職且到 Lv 30 後轉生。轉生會重置等級與裝備，所以先說明會重置／保留什麼並詢問；
// stdin 不是終端機又沒給 --yes 時拒絕。能不能轉生由 engine.CanRebirth 判斷。
func newRebirthCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rebirth",
		Short: "轉生：重新從 Lv 1 開始，保留終職能力、圖鑑與隊伍，基礎能力與經驗加成累加",
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
			now := clock.Now()
			st = engine.Apply(st, event.Event{At: now, Source: event.SourceTick, Type: "tick"}, pack).State
			if ok, reason := engine.CanRebirth(st, pack); !ok {
				return fmt.Errorf("還不能轉生：%s", reason)
			}
			r := pack.Growth.Rebirth
			next := st.Pet.Legacy.Rebirths + 1
			printf(cmd, "%s 將進行第 %d 次轉生：\n", st.Pet.Name, next)
			printf(cmd, "  重置：等級、經驗、職業、特化、進化、轉職卷軸、裝備、未鑑定背包、樓層與傳送點\n")
			printf(cmd, "  保留：種族、名字、commit 組成、圖鑑、隊伍、終職能力、飽足、心情與戰績\n")
			printf(cmd, "  轉生後：基礎能力 +%d%%、經驗 +%d%%（累加）\n", next*r.StatsPct, next*r.XPPct)
			if !yes {
				if !interactive(cmd) {
					return fmt.Errorf("轉生會重置等級與裝備，請在終端機執行，或加上 --yes")
				}
				printf(cmd, "確定要轉生嗎？[y/N] ")
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
					printf(cmd, "已取消，沒有轉生\n")
					return nil
				}
			}
			res, err := s.Update(event.Event{At: now, Source: event.SourceUser, Type: "rebirth"})
			if err != nil {
				return err
			}
			for _, l := range res.Logs {
				printf(cmd, "%s\n", l.Text)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "不詢問直接轉生")
	return cmd
}
