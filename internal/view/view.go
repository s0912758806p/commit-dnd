// Package view 把 State 與日誌排成給人看的文字（status、statusline、dungeon、class、sim、log；dex、party 在 party.go）。只讀資料、不做遊戲判斷：
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

// noPet 是還沒孵蛋時 status 與 dungeon 的提示。
const noPet = "還沒有寵物。用 dnd hatch 孵一隻吧。\n"

// Status 是 `dnd status` 的輸出：一行一個「標籤  數值」，只有文字、不畫圖。
// now 只用來判斷今天的補給還剩幾次。
func Status(s engine.State, c *content.Pack, now time.Time) string {
	p := s.Pet
	if !p.Hatched() {
		return noPet
	}
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}

	sp, _ := c.SpeciesByID(p.Species)
	row("名字", "%s", p.Name)
	row("物種", "%s", sp.Name)
	row("職業", "%s", className(p, c))
	row("進化", "%s", stageName(p, c))
	if cost := c.Cost(p.Level); cost > 0 {
		row("等級", "Lv %d（XP %d/%d）", p.Level, p.XP, cost)
	} else {
		row("等級", "Lv %d（已達最高等級）", p.Level)
	}
	row("HP  ", "%d/%d", p.HP, engine.MaxHP(p, c))
	row("飽足", "%d/%d", p.Satiety, c.Balance.Satiety.Max)
	row("心情", "%d/%d", p.Mood, c.Balance.Mood.Max)

	if left := supplyLeft(p, c, now.Format(time.DateOnly)); left > 0 {
		row("補給", "今天還剩 %d 次（XP ×%d）", left, c.Balance.Supply.Multiplier)
	} else {
		row("補給", "今天已用完")
	}
	if d := s.Dungeon; d.Floor > 0 {
		depth := floorName(d.Floor)
		if d.Deepest > d.Floor {
			depth += fmt.Sprintf("（最深 %s）", floorName(d.Deepest))
		}
		if d.Downed {
			depth += " 倒地中"
		}
		row("地城", "%s", depth)
	}
	if p.Satiety < c.Balance.Satiety.HungryBelow {
		row("狀態", "肚子餓了，commit 的 XP 減半")
	}
	return b.String()
}

// Statusline 是 `dnd statusline` 的輸出：Claude Code 底部的一行，沒有換行。
func Statusline(s engine.State, c *content.Pack, now time.Time) string {
	p := s.Pet
	if !p.Hatched() {
		return "dnd：還沒有寵物（dnd hatch）"
	}
	parts := []string{fmt.Sprintf("%s Lv %d", p.Name, p.Level)}
	if cost := c.Cost(p.Level); cost > 0 {
		parts = append(parts, fmt.Sprintf("XP %d/%d", p.XP, cost))
	}
	parts = append(parts, fmt.Sprintf("HP %d/%d", p.HP, engine.MaxHP(p, c)))
	satiety := fmt.Sprintf("飽足 %d", p.Satiety)
	if p.Satiety < c.Balance.Satiety.HungryBelow {
		satiety += "（餓）"
	}
	parts = append(parts, satiety)
	today := now.Format(time.DateOnly)
	if left := supplyLeft(p, c, today); left > 0 {
		parts = append(parts, fmt.Sprintf("補給 %d", left))
	}
	if d := s.Dungeon; d.Floor > 0 {
		floor := floorName(d.Floor)
		if d.Downed {
			floor += " 倒地"
		}
		parts = append(parts, floor)
	}
	parts = append(parts, fmt.Sprintf("今日 AI 回合 %d", s.AI.TurnsOn(today)))
	return strings.Join(parts, " · ")
}

func supplyLeft(p engine.Pet, c *content.Pack, today string) int {
	left := c.Balance.Supply.CommitsPerDay
	if p.SupplyDay == today {
		left = max(0, left-p.SupplyUsed)
	}
	return left
}

// Dungeon 是 `dnd dungeon` 的輸出：地下城的進度、眼前的怪物與裝備，格式同 Status。
func Dungeon(s engine.State, c *content.Pack) string {
	if !s.Pet.Hatched() {
		return noPet
	}
	d := s.Dungeon
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}

	var marks []string
	if d.Floor > 0 && c.BossFor(d.Floor) == "" { // Boss 層只有 Boss 一隻，不顯示進度
		marks = append(marks, fmt.Sprintf("已打倒 %d/%d", min(d.Cleared, c.Balance.Dungeon.EncountersPerFloor), c.Balance.Dungeon.EncountersPerFloor))
	}
	if d.Deepest > d.Floor {
		marks = append(marks, "最深 "+floorName(d.Deepest))
	}
	if d.Warp > 0 {
		marks = append(marks, "傳送點 "+floorName(d.Warp))
	}
	if len(marks) > 0 {
		row("樓層", "%s（%s）", floorName(d.Floor), strings.Join(marks, "，"))
	} else {
		row("樓層", "%s", floorName(d.Floor))
	}

	switch {
	case d.Monster.ID != "":
		name, boss := d.Monster.ID+" ", "" // 內容裡找不到這個 id 時退回顯示 id
		if m, ok := c.MonsterByID(d.Monster.ID); ok {
			name = m.Name + element(m.Element)
			if m.Boss {
				boss = "Boss "
			}
		}
		row("怪物", "%s%sHP %d/%d", boss, name, d.Monster.HP, d.Monster.MaxHP)
	case d.Floor == 0:
		row("怪物", "還在入口，下一個 AI 回合往下探")
	case engine.FloorCleared(s, c):
		row("怪物", "這層打完了，下一個 AI 回合往下探")
	default:
		row("怪物", "暫時沒有，下一個 AI 回合會遇到這層的下一隻")
	}
	if d.Downed {
		row("狀態", "倒地中：commit 一次就能救起來（未鑑定裝備已遺失）")
	} else {
		row("狀態", "探索中")
	}

	row("攻擊", "%d", engine.PetAtk(s, c))
	row("防禦", "%d", engine.PetDef(s, c))
	row("武器", "%s", item(d.Weapon))
	row("防具", "%s", item(d.Armor))
	row("隊伍", "%s", partySummary(s, c))
	row("背包", "未鑑定 %d/%d（commit 一次鑑定一件；倒地會全部遺失）", d.BagN, engine.BagSize)
	row("戰績", "擊敗 %d、倒地 %d", d.Kills, d.Downs)
	return b.String()
}

// floorName 把樓層排成「B12」；0 是入口。
func floorName(floor int) string {
	if floor == 0 {
		return "入口"
	}
	return fmt.Sprintf("B%d", floor)
}

// element 把屬性排成「（火）」；沒有屬性時為空字串。
func element(e string) string {
	if e == "" {
		return ""
	}
	if n, ok := content.ElementName[e]; ok {
		e = n
	}
	return "（" + e + "）"
}

// item 把裝備排成「名稱 +強度（屬性）」；沒有裝備時為「—」。
func item(it engine.Item) string {
	if it.Name == "" {
		return "—"
	}
	return fmt.Sprintf("%s +%d%s", it.Name, it.Power, element(it.Element))
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
