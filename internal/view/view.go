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

// Status 是 `dnd status` 的輸出：一行一個「標籤  數值」，只有文字、不畫圖。
// now 只用來判斷今天的補給還剩幾次。
func Status(s engine.State, c *content.Pack, now time.Time) string {
	p := s.Pet
	if !p.Hatched() {
		return "還沒有寵物。用 dnd hatch 孵一隻吧。\n"
	}
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}

	sp, _ := c.SpeciesByID(p.Species)
	class := p.Class
	if class == "" {
		class = "未轉職"
	}
	row("名字", "%s", p.Name)
	row("物種", "%s", sp.Name)
	row("職業", "%s", class)
	row("進化", "第 %d 階段", p.Stage)
	if cost := c.Cost(p.Level); cost > 0 {
		row("等級", "Lv %d（XP %d/%d）", p.Level, p.XP, cost)
	} else {
		row("等級", "Lv %d（已達最高等級）", p.Level)
	}
	row("HP  ", "%d/%d", p.HP, c.MaxHP(p.Level))
	row("飽足", "%d/%d", p.Satiety, c.Balance.Satiety.Max)
	row("心情", "%d/%d", p.Mood, c.Balance.Mood.Max)

	sup := c.Balance.Supply
	left := sup.CommitsPerDay
	if p.SupplyDay == now.Format(time.DateOnly) {
		left = max(0, left-p.SupplyUsed)
	}
	if left > 0 {
		row("補給", "今天還剩 %d 次（XP ×%d）", left, sup.Multiplier)
	} else {
		row("補給", "今天已用完")
	}
	if p.Satiety < c.Balance.Satiety.HungryBelow {
		row("狀態", "肚子餓了，commit 的 XP 減半")
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
