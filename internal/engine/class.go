package engine

// 職業、特化與進化的規則（迭代 5）。由 Apply 呼叫，全部是純函式；名稱與數值在 content/classes.yaml。
//
//   - commit 組成（Pet.Mix）：hatch 的 Args["mix"]（CLI 讀 repo 過去 90 天的 git log）當起點，
//     之後每筆計分的 commit 依類型累加。沒有 mix 參數的舊寵物靠重播已記錄的 commit 自然補算。
//   - 職業：升到 judge_level 時依 commit 組成判定（見 Judge），之後不會自己改變。
//   - 特化：到 spec_level 後，使用者用 class 事件（action=choose、spec=id）選一次；
//     等級不足、還沒轉職、不是自己職業的特化、已經選過都只寫一句日誌，狀態不變。
//   - 轉職卷軸：打倒 Boss 時有 scroll.drop_pct% 機率掉落（最多 scroll.max 張）。
//     class 事件 action=reroll 用掉一張，依當下的 commit 組成重新判定並清除特化，等級不變；
//     判定結果沒變且沒有特化可重置時不消耗卷軸。
//   - 進化：依等級決定階段（stages），升階時寫日誌。
//   - 加成：職業＋特化＋進化的百分比相加，套用在不含裝備的攻擊、防禦與 HP 上限（見 PetAtk、PetDef、MaxHP）。
//     加成改變時 HP 不會因此回復，只會被夾在新的上限內。

import (
	"strconv"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// Mix 是 commit 組成：Mix[i] 是 content.MixKinds[i] 類型的 commit 次數。
type Mix [content.MixN]int

// mixCap 是單一分類的上限，避免壞掉的 hatch 參數讓加總溢位。
const mixCap = 1 << 30

// Add 把一筆 commit 類型記進組成。
func (m *Mix) Add(kind string) {
	if i := content.MixIndex(kind); m[i] < mixCap {
		m[i]++
	}
}

// Total 是組成裡的 commit 總數。
func (m Mix) Total() int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// String 是 hatch 事件 Args["mix"] 的格式：「feat=12,fix=3,…」（依 MixKinds 順序，全部列出）。
func (m Mix) String() string {
	parts := make([]string, len(m))
	for i, v := range m {
		parts[i] = content.MixKinds[i] + "=" + strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}

// ParseMix 解析 Mix.String 的格式。不認得的類型算 other；不是非負整數的項目略過。
func ParseMix(arg string) Mix {
	var m Mix
	for kv := range strings.SplitSeq(arg, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		n, err := strconv.Atoi(v)
		if !ok || err != nil || n < 0 {
			continue
		}
		i := content.MixIndex(k)
		m[i] = min(mixCap, m[i]+min(n, mixCap))
	}
	return m
}

// Judge 依 commit 組成判定職業：分數 = 職業 kinds 的次數合計 × weight_pct，最高者當選，同分取較前面的。
// 內容沒有定義職業時回傳 false。
func Judge(m Mix, c *content.Pack) (content.Class, bool) {
	best, bestScore := -1, -1
	for i, cl := range c.Growth.Classes {
		score := 0
		for _, k := range cl.Kinds {
			score += m[content.MixIndex(k)]
		}
		score *= cl.WeightPct
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return content.Class{}, false
	}
	return c.Growth.Classes[best], true
}

// Bonus 是寵物目前的百分比加成：職業＋特化＋進化階段。
func Bonus(p Pet, c *content.Pack) content.Stats {
	var b content.Stats
	if cl, ok := c.ClassByID(p.Class); ok {
		b = b.Add(cl.Stats)
	}
	if sp, ok := c.SpecByID(p.Class, p.Spec); ok {
		b = b.Add(sp.Stats)
	}
	if st := c.Growth.Stages; p.Stage >= 1 && p.Stage <= len(st) {
		b = b.Add(st[p.Stage-1].Stats)
	}
	return b
}

// MaxHP 是寵物的 HP 上限：等級決定的基礎值乘上 HP 加成。
func MaxHP(p Pet, c *content.Pack) int {
	return max(1, pct(c.MaxHP(p.Level), 100+Bonus(p, c).HP))
}

func (r *Result) clog(at time.Time, c *content.Pack, tmpl string, vars map[string]string) {
	p := r.State.Pet
	all := map[string]string{"stage_n": strconv.Itoa(p.Stage), "stage": c.StageName(p.Species, p.Stage)}
	if cl, ok := c.ClassByID(p.Class); ok {
		all["class"], all["style"] = cl.Name, cl.Style
	}
	for k, v := range vars {
		all[k] = v
	}
	r.log(at, c, tmpl, all)
}

// grow 在每次升級後處理轉職、特化提示與進化；呼叫端之後把 HP 回滿到新的上限。
func grow(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	g := c.Growth
	if p.Class == "" && len(g.Classes) > 0 && p.Level >= g.JudgeLevel {
		cl, _ := Judge(p.Mix, c)
		p.Class = cl.ID
		r.clog(e.At, c, g.Logs.Judge, nil)
		r.Fx = append(r.Fx, EffectClass)
	}
	if p.Class != "" && p.Spec == "" && p.Level == g.SpecLevel {
		r.clog(e.At, c, g.Logs.SpecReady, nil)
	}
	if n := c.StageFor(p.Level); n > p.Stage {
		p.Stage = n
		r.clog(e.At, c, g.Logs.Evolve, nil)
		r.Fx = append(r.Fx, EffectEvolve)
	}
}

// classCmd 處理使用者的 class 事件：action=choose（spec=特化 id）或 action=reroll。
func classCmd(r *Result, e event.Event, c *content.Pack) {
	switch e.Args["action"] {
	case "choose":
		chooseSpec(r, e, c)
	case "reroll":
		reroll(r, e, c)
	}
}

func chooseSpec(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	g := c.Growth
	id := e.Args["spec"]
	deny := func(reason string) {
		r.clog(e.At, c, g.Logs.SpecDenied, map[string]string{"spec": id, "reason": reason})
	}
	cl, hasClass := c.ClassByID(p.Class)
	sp, ok := c.SpecByID(p.Class, id)
	switch {
	case !hasClass:
		deny("還沒轉職（Lv " + strconv.Itoa(g.JudgeLevel) + " 依 commit 組成判定）")
	case p.Level < g.SpecLevel:
		deny("Lv " + strconv.Itoa(g.SpecLevel) + " 才能選擇特化")
	case p.Spec != "":
		cur, _ := c.SpecByID(p.Class, p.Spec)
		deny("已經選了「" + cur.Name + "」，要換請用轉職卷軸")
	case !ok:
		deny(cl.Name + "沒有這個特化")
	default:
		p.Spec = sp.ID
		p.HP = min(p.HP, MaxHP(*p, c))
		r.clog(e.At, c, g.Logs.SpecChosen, map[string]string{"spec": sp.Name})
	}
}

func reroll(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	g := c.Growth
	deny := func(reason string) {
		r.clog(e.At, c, g.Logs.RerollDenied, map[string]string{"reason": reason})
	}
	next, _ := Judge(p.Mix, c)
	switch {
	case p.Class == "":
		deny("還沒轉職（Lv " + strconv.Itoa(g.JudgeLevel) + " 依 commit 組成判定）")
	case p.Scrolls <= 0:
		deny("沒有轉職卷軸（打倒 Boss 有機會掉落）")
	case next.ID == p.Class && p.Spec == "":
		deny("依目前的 commit 組成還是" + next.Name + "，卷軸沒有用掉")
	default:
		p.Scrolls--
		same := next.ID == p.Class
		p.Class, p.Spec = next.ID, ""
		p.HP = min(p.HP, MaxHP(*p, c))
		if same {
			r.clog(e.At, c, g.Logs.RerollSame, nil)
		} else {
			r.clog(e.At, c, g.Logs.Reroll, nil)
		}
		r.Fx = append(r.Fx, EffectClass)
	}
}

// dropScroll 在打倒 Boss 時依機率掉落轉職卷軸；呼叫時眼前的怪物還是那隻 Boss。
func dropScroll(r *Result, e event.Event, c *content.Pack) {
	s := c.Growth.Scroll
	rr := rng{seedFor(r.State, e.Seq, "scroll")}
	if s.DropPct <= 0 || rr.intn(100) >= s.DropPct {
		return
	}
	p := &r.State.Pet
	vars := map[string]string{"scroll_max": strconv.Itoa(s.Max)}
	if p.Scrolls >= s.Max {
		r.dlog(e.At, c, c.Growth.Logs.ScrollFull, vars)
		return
	}
	p.Scrolls++
	vars["scrolls"] = strconv.Itoa(p.Scrolls)
	r.dlog(e.At, c, c.Growth.Logs.ScrollDrop, vars)
}
