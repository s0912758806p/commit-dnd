// Package view 把 State 與日誌排成給人看的文字。只讀資料、不做遊戲判斷：
// 呼叫端負責先套用 tick（status 看到的是「現在」的狀態），這裡只負責排版。
// 輸出格式由 testdata/golden/*.txt 守住，改版面時用 `go test ./internal/view -update` 更新。
package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

const barWidth = 10

// Status 是 `dnd status` 的輸出。now 只用來判斷今天的補給還剩幾次。
func Status(s engine.State, c *content.Pack, now time.Time) string {
	p := s.Pet
	if !p.Hatched() {
		return "還沒有寵物。用 dnd hatch 孵一隻吧。\n"
	}
	var b strings.Builder
	sp, _ := c.SpeciesByID(p.Species)
	b.WriteString(sp.Art)
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s（%s）Lv %d\n", p.Name, sp.Name, p.Level)

	if cost := c.Cost(p.Level); cost > 0 {
		fmt.Fprintf(&b, "XP   %s  %d/%d\n", bar(p.XP, cost), p.XP, cost)
	} else {
		fmt.Fprintf(&b, "XP   %s  已達最高等級\n", bar(1, 1))
	}
	maxHP := c.MaxHP(p.Level)
	fmt.Fprintf(&b, "HP   %s  %d/%d\n", bar(p.HP, maxHP), p.HP, maxHP)
	fmt.Fprintf(&b, "飽足 %s  %d/%d\n", bar(p.Satiety, c.Balance.Satiety.Max), p.Satiety, c.Balance.Satiety.Max)
	fmt.Fprintf(&b, "心情 %s  %d/%d\n", bar(p.Mood, c.Balance.Mood.Max), p.Mood, c.Balance.Mood.Max)

	sup := c.Balance.Supply
	left := sup.CommitsPerDay
	if p.SupplyDay == now.Format(time.DateOnly) {
		left = max(0, left-p.SupplyUsed)
	}
	if left > 0 {
		fmt.Fprintf(&b, "今日補給：接下來 %d 筆 commit 的 XP ×%d\n", left, sup.Multiplier)
	} else {
		b.WriteString("今日補給：已用完，明天再來\n")
	}
	if p.Satiety < c.Balance.Satiety.HungryBelow {
		b.WriteString("肚子餓了：commit 的 XP 減半，多 commit 餵牠吧。\n")
	}
	return b.String()
}

// Log 是 `dnd log` 的輸出：每行「月/日 時:分  日誌」，時間用事件發生時的時區。
func Log(lines []engine.LogLine) string {
	if len(lines) == 0 {
		return "還沒有任何冒險紀錄。\n"
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%s  %s\n", l.At.Format("01/02 15:04"), l.Text)
	}
	return b.String()
}

func bar(v, limit int) string {
	n := 0
	if limit > 0 {
		n = min(barWidth, max(0, v*barWidth/limit))
	}
	return strings.Repeat("█", n) + strings.Repeat("░", barWidth-n)
}
