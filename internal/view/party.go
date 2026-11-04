package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

// Dex 是 `dnd dex` 的輸出：依 monsters.yaml 的順序列出所有怪物，
// 收服過的、遇過但還沒收服的、沒遇過的（名稱與 id 都顯示 ???）。
func Dex(s engine.State, c *content.Pack) string {
	if !s.Pet.Hatched() {
		return noPet
	}
	normal, caught, seen := 0, 0, 0
	for _, m := range c.Monsters {
		en, ok := s.Dex.Lookup(m.ID)
		if !m.Boss {
			normal++
			if en.Caught > 0 {
				caught++
			}
		}
		if ok && en.Seen+en.Caught > 0 {
			seen++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "圖鑑  收服 %d/%d 種一般怪物，遇過 %d/%d 種\n", caught, normal, seen, len(c.Monsters))
	for i, m := range c.Monsters {
		en, _ := s.Dex.Lookup(m.ID)
		no := fmt.Sprintf("No.%02d", i+1)
		if en.Seen+en.Caught == 0 {
			fmt.Fprintf(&b, "%s  ???\n", no)
			continue
		}
		var note string
		switch {
		case m.Boss:
			note = fmt.Sprintf("Boss，遇過 %d 次（無法收服）", en.Seen)
		case en.Caught > 0:
			note = fmt.Sprintf("收服 %d 次（最深 %s）", en.Caught, floorName(en.BestFloor))
			if s.Party.Has(m.ID) {
				note += "，隊伍中"
			}
		default:
			note = fmt.Sprintf("遇過 %d 次，還沒收服", en.Seen)
		}
		fmt.Fprintf(&b, "%s  %s%s %s，%s\n", no, m.Name, element(m.Element), m.ID, note)
	}
	return b.String()
}

// Party 是 `dnd party` 的輸出：隊員與加成、可以換上的候補、合體技是否就緒。
// now 只用來判斷合體技的時間窗。
func Party(s engine.State, c *content.Pack, now time.Time) string {
	if !s.Pet.Hatched() {
		return noPet
	}
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}
	p := s.Party
	if p.N == 0 {
		row("隊伍", "0/%d，還沒有隊員（fix: commit 時有機會收服眼前的一般怪物）", engine.PartySize)
	} else {
		row("隊伍", "%d/%d，攻擊 +%d", p.N, engine.PartySize, engine.PartyAtk(s, c))
	}
	for _, id := range p.Members[:p.N] {
		en, _ := s.Dex.Lookup(id)
		row("隊員", "%s，最深 %s 收服，攻擊 +%d", monsterLabel(id, c), floorName(en.BestFloor), engine.MemberAtk(s, c, id))
	}
	var bench []string
	for _, m := range c.Monsters {
		if en, _ := s.Dex.Lookup(m.ID); en.Caught > 0 && !p.Has(m.ID) {
			bench = append(bench, fmt.Sprintf("%s 攻擊 +%d", monsterLabel(m.ID, c), engine.MemberAtk(s, c, m.ID)))
		}
	}
	if len(bench) > 0 {
		row("候補", "%s", strings.Join(bench, "、"))
	}

	window := c.Balance.Party.ComboWindowMinutes
	if engine.ComboReady(s, now, c) {
		row("合體技", "就緒：現在 commit 會對眼前的怪物造成攻擊 ×%d%% 的傷害（最後 AI 活動 %s，%d 分鐘內有效）",
			engine.ComboPct(s, c), p.LastAI.Format("01/02 15:04"), window)
	} else {
		row("合體技", "未就緒：commit 前 %d 分鐘內有 AI 活動才會發動（攻擊 ×%d%%）", window, engine.ComboPct(s, c))
	}
	return b.String()
}

// partySummary 是 `dnd dungeon` 的隊伍那一行。
func partySummary(s engine.State, c *content.Pack) string {
	p := s.Party
	if p.N == 0 {
		return fmt.Sprintf("0/%d（fix: commit 時有機會收服眼前的怪物）", engine.PartySize)
	}
	names := make([]string, p.N)
	for i, id := range p.Members[:p.N] {
		names[i] = id
		if m, ok := c.MonsterByID(id); ok {
			names[i] = m.Name
		}
	}
	return fmt.Sprintf("%s（%d/%d，攻擊 +%d）", strings.Join(names, "、"), p.N, engine.PartySize, engine.PartyAtk(s, c))
}

// monsterLabel 把怪物排成「名稱（屬性）id」；內容裡找不到時只顯示 id。
func monsterLabel(id string, c *content.Pack) string {
	m, ok := c.MonsterByID(id)
	if !ok {
		return id
	}
	return m.Name + element(m.Element) + " " + id
}
