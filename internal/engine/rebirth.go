package engine

// 終職與轉生的規則（迭代 6）。由 Apply 呼叫，全部是純函式；名稱與數值在 content/classes.yaml。
//
//   - 終職：等級 ≥ final_level 且已選特化時，依特化取得對應的終職（每一輪只取得一次；
//     Lv 30 還沒選特化的話，選特化的那一刻取得）。終職記進 Pet.Legacy，等級 +1：
//     第一次取得是 Lv 1，之後的輪迴再取得同一個終職就強化一級。
//   - 終職能力：被動、跨轉生保留；效果 = per_level × 終職等級，由 Ability 查詢，散在各規則裡套用。
//     能力只強化 commit 與玩法，**不會讓 AI 回血或免除倒地**（DEVPLAN 鐵律，見 TestAbilitiesNeverLetAIHeal）。
//   - 轉生：取得這一輪的終職且等級 ≥ rebirth.level 時，使用者的 rebirth 事件才會成功（倒地中不行）。
//     重置等級、XP、職業、特化、進化、卷軸、裝備、背包、樓層與傳送點；保留種族、名字、commit 組成、
//     圖鑑、隊伍、Legacy，以及飽足、心情、補給與戰績。每次轉生累加基礎能力 stats_pct% 與 commit XP xp_pct%。

import (
	"strconv"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// Legacy 是跨轉生保留的成就：轉生次數與每個終職的等級（依第一次取得的順序）。
type Legacy struct {
	Rebirths int                           `json:"rebirths"`
	Finals   [content.MaxFinals]FinalEntry `json:"finals"`
	FinalN   int                           `json:"final_n"`
}

type FinalEntry struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
}

// FinalLevel 回傳某個終職目前的等級；沒取得過為 0。
func (l Legacy) FinalLevel(id string) int {
	for _, f := range l.Finals[:l.FinalN] {
		if f.ID == id {
			return f.Level
		}
	}
	return 0
}

// Ability 回傳某種終職能力的總效果（所有取得過的終職加總）。kind_xp_pct 請用 KindXPPct。
func Ability(p Pet, c *content.Pack, ability string) int {
	total := 0
	for _, en := range p.Legacy.Finals[:p.Legacy.FinalN] {
		if f, ok := c.FinalByID(en.ID); ok && f.Ability == ability {
			total += f.PerLevel * en.Level
		}
	}
	return total
}

// KindXPPct 回傳某類型 commit 的額外 XP 百分比（kind_xp_pct 能力）。
func KindXPPct(p Pet, c *content.Pack, kind string) int {
	total := 0
	for _, en := range p.Legacy.Finals[:p.Legacy.FinalN] {
		if f, ok := c.FinalByID(en.ID); ok && f.Ability == "kind_xp_pct" && f.Kind == kind {
			total += f.PerLevel * en.Level
		}
	}
	return total
}

// RebirthStats 是轉生累加的基礎能力加成（攻擊、防禦、HP 上限的百分比）。
func RebirthStats(p Pet, c *content.Pack) content.Stats {
	n := p.Legacy.Rebirths * c.Growth.Rebirth.StatsPct
	return content.Stats{Atk: n, Def: n, HP: n}
}

// RebirthXPPct 是轉生累加的 commit XP 加成（百分比）。
func RebirthXPPct(p Pet, c *content.Pack) int {
	return p.Legacy.Rebirths * c.Growth.Rebirth.XPPct
}

// promote 在條件滿足時讓寵物取得這一輪的終職；grow（升級）與選特化之後呼叫。
func promote(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	g := c.Growth
	if p.Final != "" || p.Spec == "" || g.FinalLevel == 0 || p.Level < g.FinalLevel {
		return
	}
	f, ok := c.FinalForSpec(p.Spec)
	if !ok {
		return
	}
	l := &p.Legacy
	i := 0
	for i < l.FinalN && l.Finals[i].ID != f.ID {
		i++
	}
	if i == l.FinalN {
		if l.FinalN == len(l.Finals) {
			return // 內容驗證保證終職種類不超過容量
		}
		l.Finals[i] = FinalEntry{ID: f.ID}
		l.FinalN++
	}
	l.Finals[i].Level++
	p.Final = f.ID
	level := l.Finals[i].Level
	tmpl := g.Logs.Final
	if level > 1 {
		tmpl = g.Logs.FinalUp
	}
	r.clog(e.At, c, tmpl, map[string]string{"final": f.Name, "final_level": strconv.Itoa(level), "ability": f.AbilityText(level)})
	r.Fx = append(r.Fx, EffectFinal)
}

// CanRebirth 回報現在能不能轉生；不能時回傳原因。引擎與顯示共用這個判斷。
func CanRebirth(s State, c *content.Pack) (bool, string) {
	g := c.Growth
	switch {
	case !s.Pet.Hatched():
		return false, "還沒有寵物"
	case s.Pet.Final == "":
		return false, "要先在 Lv " + strconv.Itoa(g.FinalLevel) + " 取得終職（需要已選擇特化）"
	case s.Pet.Level < g.Rebirth.Level:
		return false, "要到 Lv " + strconv.Itoa(g.Rebirth.Level)
	case s.Dungeon.Downed:
		return false, "倒地中不能轉生，先 commit 把牠救起來"
	}
	return true, ""
}

// rebirth 處理使用者的轉生事件。
func rebirth(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	g := c.Growth
	switch ok, reason := CanRebirth(r.State, c); {
	case !ok:
		r.clog(e.At, c, g.Logs.RebirthDenied, map[string]string{"reason": reason})
	default:
		old := *p
		legacy := old.Legacy
		legacy.Rebirths++
		*p = Pet{
			Species: old.Species, Name: old.Name, Stage: 1, Level: 1, Mix: old.Mix, Legacy: legacy,
			Satiety: old.Satiety, Mood: old.Mood, Pets: old.Pets, Commits: old.Commits,
			SupplyDay: old.SupplyDay, SupplyUsed: old.SupplyUsed,
			HPCarry: old.HPCarry, SatietyCarry: old.SatietyCarry, MoodCarry: old.MoodCarry,
		}
		p.HP = MaxHP(*p, c)
		d := &r.State.Dungeon
		*d = Dungeon{Kills: d.Kills, Downs: d.Downs, LastActive: d.LastActive}
		r.clog(e.At, c, g.Logs.Rebirth, map[string]string{
			"rebirths": strconv.Itoa(legacy.Rebirths),
			"stats":    strconv.Itoa(RebirthStats(*p, c).Atk),
			"xp":       strconv.Itoa(RebirthXPPct(*p, c)),
		})
		r.Fx = append(r.Fx, EffectRebirth)
	}
}
