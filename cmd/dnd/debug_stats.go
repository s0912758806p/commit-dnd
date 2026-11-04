package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd debug stats`：重播 events.jsonl，依事件的當地日期（和每日補給同一個日期）列出每天的
// XP、計分的 commit、AI 回合、倒地次數、到過的最深樓層與 AI 回合下探的層數，最後一行總計與平均每天下探幾層。
// 只讀、不取鎖；數字都從事件前後的 State 差值算出，不另外解析事件內容，和引擎的判斷永遠一致。
func newDebugStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "從 events.jsonl 統計每天的 XP、commit、AI 回合、倒地與下探層數",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, pack, err := openStore()
			if err != nil {
				return err
			}
			days := map[string]*dayStats{}
			err = s.Walk(func(e event.Event, before engine.State, r engine.Result) {
				day := e.At.Format(time.DateOnly)
				d := days[day]
				if d == nil {
					d = &dayStats{Day: day}
					days[day] = d
				}
				d.add(e, before, r.State, pack)
			})
			if err != nil {
				return err
			}
			printf(cmd, "%s", renderStats(days))
			return nil
		},
	}
}

type dayStats struct {
	Day                                string
	XP, Commits, Turns, Downs, Deepest int
	Descended                          int // AI 回合往下探的層數（不含 warp、debug floor）
}

func (d *dayStats) add(e event.Event, before, after engine.State, c *content.Pack) {
	d.XP += totalXP(after.Pet, c) - totalXP(before.Pet, c)
	d.Commits += after.Pet.Commits - before.Pet.Commits
	d.Turns += after.AI.Turns - before.AI.Turns
	d.Downs += after.Dungeon.Downs - before.Dungeon.Downs
	d.Deepest = max(d.Deepest, before.Dungeon.Floor, after.Dungeon.Floor)
	if e.Type == "turn" && after.Dungeon.Floor > before.Dungeon.Floor {
		d.Descended += after.Dungeon.Floor - before.Dungeon.Floor
	}
}

// totalXP 是孵蛋以來累積的 XP：已升過的每級成本加上本級的 XP。
func totalXP(p engine.Pet, c *content.Pack) int {
	xp := p.XP
	for l := 1; l < p.Level; l++ {
		xp += c.Cost(l)
	}
	return xp
}

var statsHeader = []string{"日期", "XP", "commit", "AI 回合", "倒地", "最深", "下探"}

func renderStats(days map[string]*dayStats) string {
	if len(days) == 0 {
		return "events.jsonl 沒有事件。\n"
	}
	keys := make([]string, 0, len(days))
	for k := range days {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := [][]string{statsHeader}
	var sum dayStats
	for _, k := range keys {
		d := days[k]
		rows = append(rows, d.row(d.Day))
		sum.XP += d.XP
		sum.Commits += d.Commits
		sum.Turns += d.Turns
		sum.Downs += d.Downs
		sum.Descended += d.Descended
		sum.Deepest = max(sum.Deepest, d.Deepest)
	}
	rows = append(rows, sum.row("總計"))

	widths := make([]int, len(statsHeader))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], displayWidth(cell))
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			pad := strings.Repeat(" ", widths[i]-displayWidth(cell))
			if i == 0 {
				b.WriteString(cell + pad) // 日期靠左，數字靠右
			} else {
				b.WriteString("  " + pad + cell)
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "共 %d 天，平均每天下探 %.1f 層\n", len(keys), float64(sum.Descended)/float64(len(keys)))
	return b.String()
}

func (d dayStats) row(label string) []string {
	deepest := "入口"
	if d.Deepest > 0 {
		deepest = fmt.Sprintf("B%d", d.Deepest)
	}
	return []string{label, fmt.Sprint(d.XP), fmt.Sprint(d.Commits), fmt.Sprint(d.Turns), fmt.Sprint(d.Downs), deepest, fmt.Sprint(d.Descended)}
}

// displayWidth 估算終端機顯示寬度：全形字（中文）算 2 格。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 {
			w += 2
		} else {
			w++
		}
	}
	return w
}
