package view

import (
	"fmt"
	"slices"
	"strings"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

// Class 是 `dnd class` 的輸出：職業、特化、進化、加成、卷軸與 commit 組成，接著列出可選的方向
// （還沒轉職時列出所有職業，轉職後列出這個職業的特化）。格式同 Status。
func Class(s engine.State, c *content.Pack) string {
	p := s.Pet
	if !p.Hatched() {
		return noPet
	}
	g := c.Growth
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}

	cl, hasClass := c.ClassByID(p.Class)
	if hasClass {
		row("職業", "%s（%s）", cl.Name, cl.Style)
	} else {
		row("職業", "未轉職（Lv %d 依 commit 組成判定）", g.JudgeLevel)
	}
	switch sp, ok := c.SpecByID(p.Class, p.Spec); {
	case ok:
		row("特化", "%s", sp.Name)
	case hasClass && p.Level >= g.SpecLevel:
		row("特化", "可以選擇了：dnd class choose <特化 id>")
	default:
		row("特化", "尚未選擇（Lv %d 可以選擇）", g.SpecLevel)
	}
	if next := p.Stage + 1; next <= len(g.Stages) {
		row("進化", "%s，Lv %d 進化成%s", stageName(p, c), g.Stages[next-1].Level, c.StageName(p.Species, next))
	} else {
		row("進化", "%s，已是最終階段", stageName(p, c))
	}
	switch f, ok := c.FinalByID(p.Final); {
	case ok:
		row("終職", "%s（這一輪已取得）", f.Name)
	case p.Spec != "":
		row("終職", "Lv %d 取得", g.FinalLevel)
	default:
		row("終職", "Lv %d、選好特化後取得", g.FinalLevel)
	}
	row("加成", "%s（職業＋特化＋進化＋轉生，加在不含裝備的能力上）", stats(engine.Bonus(p, c)))
	if n := p.Legacy.Rebirths; n > 0 {
		row("轉生", "%d 次（基礎能力 +%d%%、經驗 +%d%%）", n, engine.RebirthStats(p, c).Atk, engine.RebirthXPPct(p, c))
	}
	if ok, reason := engine.CanRebirth(s, c); ok {
		row("轉生", "可以轉生了：dnd rebirth")
	} else if p.Final != "" {
		row("轉生", "還不能轉生：%s", reason)
	}
	if p.Scrolls > 0 {
		row("卷軸", "轉職卷軸 %d/%d 張（dnd class reroll 依目前的 commit 組成重新判定）", p.Scrolls, g.Scroll.Max)
	} else {
		row("卷軸", "轉職卷軸 0/%d 張（打倒 Boss 有機會掉落）", g.Scroll.Max)
	}
	row("組成", "%s", Mix(p.Mix))
	if j, ok := engine.Judge(p.Mix, c); ok && p.Mix.Total() > 0 {
		row("判定", "依目前的組成會是%s（%s）", j.Name, j.Style)
	}

	if hasClass {
		fmt.Fprintf(&b, "\n%s的特化：\n", cl.Name)
		for _, sp := range cl.Specs {
			mark := ""
			if sp.ID == p.Spec {
				mark = " ← 已選"
			}
			fmt.Fprintf(&b, "  %s：%s（%s）%s\n", sp.ID, sp.Name, stats(sp.Stats), mark)
			if f, ok := c.FinalForSpec(sp.ID); ok {
				next := p.Legacy.FinalLevel(f.ID) + 1 // 下一次取得時的等級
				fmt.Fprintf(&b, "      → 終職 %s Lv %d：%s\n", f.Name, next, f.AbilityText(next))
			}
		}
		legacy(&b, p, c)
		return b.String()
	}
	b.WriteString("\n職業一覽（commit 組成裡哪一類明顯偏多，就成為哪個職業）：\n")
	for _, cl := range g.Classes {
		fmt.Fprintf(&b, "  %s（%s）：%s 多（%s）\n", cl.Name, cl.Style, strings.Join(cl.Kinds, "／"), stats(cl.Stats))
	}
	legacy(&b, p, c)
	return b.String()
}

// legacy 列出跨轉生保留的終職能力；還沒有任何終職時不輸出。
func legacy(b *strings.Builder, p engine.Pet, c *content.Pack) {
	l := p.Legacy
	if l.FinalN == 0 {
		return
	}
	b.WriteString("\n終職能力（跨轉生保留，同一終職再次取得會強化）：\n")
	for _, en := range l.Finals[:l.FinalN] {
		if f, ok := c.FinalByID(en.ID); ok {
			fmt.Fprintf(b, "  %s Lv %d：%s\n", f.Name, en.Level, f.AbilityText(en.Level))
		}
	}
}

// className 是 status 的職業欄：「遊俠（開拓型）」，選了特化時接上「・神射手」。
func className(p engine.Pet, c *content.Pack) string {
	cl, ok := c.ClassByID(p.Class)
	if !ok {
		if p.Class != "" { // 內容裡找不到這個 id 時退回顯示 id
			return p.Class
		}
		return "未轉職"
	}
	name := fmt.Sprintf("%s（%s）", cl.Name, cl.Style)
	if sp, ok := c.SpecByID(p.Class, p.Spec); ok {
		name += "・" + sp.Name
	}
	if f, ok := c.FinalByID(p.Final); ok {
		name += "・終職" + f.Name
	}
	return name
}

// stageName 是「第 2 階段（勇者）」；內容沒有這個物種的名稱時只顯示階段。
func stageName(p engine.Pet, c *content.Pack) string {
	if n := c.StageName(p.Species, p.Stage); n != "" {
		return fmt.Sprintf("第 %d 階段（%s）", p.Stage, n)
	}
	return fmt.Sprintf("第 %d 階段", p.Stage)
}

// stats 把百分比加成排成「攻擊 +20%、HP +10%」，略過 0；全部是 0 時為「無」。
func stats(s content.Stats) string {
	var parts []string
	for _, f := range []struct {
		name string
		v    int
	}{{"攻擊", s.Atk}, {"防禦", s.Def}, {"HP", s.HP}} {
		if f.v != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d%%", f.name, f.v))
		}
	}
	if len(parts) == 0 {
		return "無"
	}
	return strings.Join(parts, "、")
}

// Mix 把 commit 組成排成「feat 12（40%）、docs 6（20%）…，共 30 筆」，由多到少、略過 0。
func Mix(m engine.Mix) string {
	total := m.Total()
	if total == 0 {
		return "還沒有紀錄（之後的 commit 會累加）"
	}
	idx := make([]int, 0, len(m))
	for i, v := range m {
		if v > 0 {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(a, b int) int { return m[b] - m[a] }) // 多的在前，同數時依 MixKinds 順序
	parts := make([]string, len(idx))
	for i, k := range idx {
		parts[i] = fmt.Sprintf("%s %d（%d%%）", content.MixKinds[k], m[k], m[k]*100/total)
	}
	return strings.Join(parts, "、") + fmt.Sprintf("，共 %d 筆", total)
}
