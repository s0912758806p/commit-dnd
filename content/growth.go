package content

// classes.yaml：職業、特化、進化與轉職卷軸（迭代 5，規則在 internal/engine/class.go），
// 終職與轉生（迭代 6，規則在 internal/engine/rebirth.go）。

import (
	"fmt"
	"slices"
)

// MixKinds 是 commit 組成（commit_mix）的固定分類，順序就是 engine.Mix 陣列的索引。
// 不在清單裡的 commit 類型（style、chore、ci…）一律算 other（最後一個）。
var MixKinds = [MixN]string{"feat", "fix", "test", "refactor", "perf", "docs", "other"}

// MixN 是 commit 組成的分類數；engine.Mix 是 [MixN]int，State 才能保持可比較。
const MixN = 7

// MixIndex 回傳 commit 類型在 MixKinds 裡的索引；不認得的類型回傳 other 的索引。
func MixIndex(kind string) int {
	if i := slices.Index(MixKinds[:], kind); i >= 0 {
		return i
	}
	return MixN - 1
}

// Stats 是百分比加成：攻擊、防禦、HP 上限。
type Stats struct {
	Atk int `yaml:"atk"`
	Def int `yaml:"def"`
	HP  int `yaml:"hp"`
}

// Add 回傳兩組加成的和。
func (s Stats) Add(o Stats) Stats { return Stats{s.Atk + o.Atk, s.Def + o.Def, s.HP + o.HP} }

type Growth struct {
	JudgeLevel int                 `yaml:"judge_level"`
	SpecLevel  int                 `yaml:"spec_level"`
	Classes    []Class             `yaml:"classes"`
	Stages     []Stage             `yaml:"stages"`
	StageNames map[string][]string `yaml:"stage_names"` // 物種 id → 各階段名稱
	Scroll     Scroll              `yaml:"scroll"`
	FinalLevel int                 `yaml:"final_level"`
	Finals     []Final             `yaml:"finals"`
	Rebirth    Rebirth             `yaml:"rebirth"`
	Logs       GrowthLogs          `yaml:"logs"`
}

// Final 是終職：由特化 Spec 決定，帶一種被動能力，效果 = PerLevel × 終職等級。
type Final struct {
	ID       string `yaml:"id"`
	Spec     string `yaml:"spec"`
	Name     string `yaml:"name"`
	Ability  string `yaml:"ability"`
	PerLevel int    `yaml:"per_level"`
	Kind     string `yaml:"kind"` // 只有 kind_xp_pct 使用：哪一類 commit
}

// Rebirth 是轉生的條件與每次轉生累加的加成。
type Rebirth struct {
	Level    int `yaml:"level"`
	StatsPct int `yaml:"stats_pct"`
	XPPct    int `yaml:"xp_pct"`
}

// MaxFinals 是終職種類的上限；engine.Legacy 用固定大小陣列記錄，State 才能保持可比較。
const MaxFinals = 32

// abilityText 是各種能力的說明格式（%d 代入效果數值）；同時是合法能力的清單。
var abilityText = map[string]string{
	"combo_pct":      "合體技倍率 +%d%%",
	"satiety_slow":   "飽足下降速度 −%d%%",
	"capture_pct":    "收服機率 +%d%%",
	"rescue_hp_pct":  "commit 救起時的 HP +%d%%",
	"kind_xp_pct":    "%s commit 的經驗 +%d%%",
	"keep_on_down":   "倒地時保留 %d 件未鑑定裝備",
	"item_power":     "鑑定出的裝備強度 +%d",
	"commit_heal":    "每筆 commit 多回復 %d HP",
	"element_pct":    "屬性相剋時傷害 +%d%%",
	"supply_commits": "每天的補給次數 +%d",
	"party_pct":      "隊伍攻擊加成 +%d%%",
	"scroll_pct":     "轉職卷軸掉落機率 +%d%%",
}

// AbilityText 回傳終職在某等級的能力說明，例如「合體技倍率 +100%」。
func (f Final) AbilityText(level int) string {
	if f.Ability == "kind_xp_pct" {
		return fmt.Sprintf(abilityText[f.Ability], f.Kind, f.PerLevel*level)
	}
	return fmt.Sprintf(abilityText[f.Ability], f.PerLevel*level)
}

type Class struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	Style     string   `yaml:"style"` // 開拓型、修補型……
	Kinds     []string `yaml:"kinds"` // 計分的 commit 組成分類
	WeightPct int      `yaml:"weight_pct"`
	Stats     `yaml:",inline"`
	Specs     []Spec `yaml:"specs"`
}

type Spec struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Stats `yaml:",inline"`
}

// Stage 是進化階段：從 Level 起進入這個階段。
type Stage struct {
	Level int `yaml:"level"`
	Stats `yaml:",inline"`
}

type Scroll struct {
	DropPct int `yaml:"drop_pct"`
	Max     int `yaml:"max"`
}

type GrowthLogs struct {
	Judge         string `yaml:"judge"`
	SpecReady     string `yaml:"spec_ready"`
	SpecChosen    string `yaml:"spec_chosen"`
	SpecDenied    string `yaml:"spec_denied"`
	Evolve        string `yaml:"evolve"`
	ScrollDrop    string `yaml:"scroll_drop"`
	ScrollFull    string `yaml:"scroll_full"`
	Reroll        string `yaml:"reroll"`
	RerollSame    string `yaml:"reroll_same"`
	RerollDenied  string `yaml:"reroll_denied"`
	Final         string `yaml:"final"`
	FinalUp       string `yaml:"final_up"`
	Rebirth       string `yaml:"rebirth"`
	RebirthDenied string `yaml:"rebirth_denied"`
}

func (p *Pack) validateGrowth() error {
	g := p.Growth
	if g.JudgeLevel < 2 || g.SpecLevel <= g.JudgeLevel || len(g.Classes) == 0 {
		return fmt.Errorf("classes.yaml: 需 2 ≤ judge_level < spec_level，且至少一個職業")
	}
	ids := map[string]bool{}
	covered := map[string]bool{}
	for _, c := range g.Classes {
		if c.ID == "" || c.Name == "" || c.Style == "" || ids[c.ID] || c.WeightPct < 1 || len(c.Kinds) == 0 || len(c.Specs) == 0 {
			return fmt.Errorf("classes.yaml: 職業 %q 的 id／name／style／kinds／weight_pct／specs 不正確或 id 重複", c.ID)
		}
		ids[c.ID] = true
		for _, k := range c.Kinds {
			if !slices.Contains(MixKinds[:], k) || covered[k] {
				return fmt.Errorf("classes.yaml: 職業 %q 的 kinds %q 不是 commit 組成的分類，或已被其他職業使用", c.ID, k)
			}
			covered[k] = true
		}
		for _, s := range c.Specs {
			if s.ID == "" || s.Name == "" || ids[s.ID] {
				return fmt.Errorf("classes.yaml: 特化 %q 的 id／name 不正確或 id 重複", s.ID)
			}
			ids[s.ID] = true
		}
	}
	if len(g.Stages) == 0 || g.Stages[0].Level != 1 {
		return fmt.Errorf("classes.yaml: stages 必須從 level: 1 開始")
	}
	for i := 1; i < len(g.Stages); i++ {
		if g.Stages[i].Level <= g.Stages[i-1].Level {
			return fmt.Errorf("classes.yaml: stages 的 level 必須遞增")
		}
	}
	for _, sp := range p.Species {
		if names := g.StageNames[sp.ID]; len(names) != len(g.Stages) || slices.Contains(names, "") {
			return fmt.Errorf("classes.yaml: stage_names.%s 要有 %d 個名稱", sp.ID, len(g.Stages))
		}
	}
	if g.Scroll.DropPct < 0 || g.Scroll.DropPct > 100 || g.Scroll.Max < 1 {
		return fmt.Errorf("classes.yaml: scroll 需 0 ≤ drop_pct ≤ 100、max ≥ 1")
	}
	if err := p.validateFinals(); err != nil {
		return err
	}
	l := g.Logs
	for _, s := range []string{l.Judge, l.SpecReady, l.SpecChosen, l.SpecDenied, l.Evolve, l.ScrollDrop, l.ScrollFull, l.Reroll, l.RerollSame, l.RerollDenied,
		l.Final, l.FinalUp, l.Rebirth, l.RebirthDenied} {
		if s == "" {
			return fmt.Errorf("classes.yaml: logs 的每一句都要有內容")
		}
	}
	return nil
}

// ClassByID 找出職業；找不到回傳 false。
func (p *Pack) ClassByID(id string) (Class, bool) {
	for _, c := range p.Growth.Classes {
		if c.ID == id {
			return c, true
		}
	}
	return Class{}, false
}

// SpecByID 找出某職業的特化；職業不存在或沒有這個特化時回傳 false。
func (p *Pack) SpecByID(class, spec string) (Spec, bool) {
	c, _ := p.ClassByID(class)
	for _, s := range c.Specs {
		if s.ID == spec {
			return s, true
		}
	}
	return Spec{}, false
}

// StageFor 回傳某等級的進化階段（從 1 起算）；沒有定義階段時為 1。
func (p *Pack) StageFor(level int) int {
	n := 1
	for i, s := range p.Growth.Stages {
		if s.Level <= level {
			n = i + 1
		}
	}
	return n
}

// StageName 回傳某物種（可用舊 id）在某階段的名稱；沒有定義時為空字串。
func (p *Pack) StageName(species string, stage int) string {
	if sp, ok := p.SpeciesByID(species); ok {
		species = sp.ID
	}
	names := p.Growth.StageNames[species]
	if stage < 1 || stage > len(names) {
		return ""
	}
	return names[stage-1]
}

// validateFinals：每個特化剛好一個終職、能力合法；轉生門檻不低於終職等級。
func (p *Pack) validateFinals() error {
	g := p.Growth
	if g.FinalLevel <= g.SpecLevel || len(g.Finals) > MaxFinals {
		return fmt.Errorf("classes.yaml: 需 final_level > spec_level，終職最多 %d 種", MaxFinals)
	}
	if g.Rebirth.Level < g.FinalLevel || g.Rebirth.StatsPct < 0 || g.Rebirth.XPPct < 0 {
		return fmt.Errorf("classes.yaml: rebirth.level 不可低於 final_level，加成不可為負")
	}
	ids, specs := map[string]bool{}, map[string]bool{}
	for _, f := range g.Finals {
		_, known := abilityText[f.Ability]
		if f.ID == "" || f.Name == "" || ids[f.ID] || specs[f.Spec] || !known || f.PerLevel < 1 ||
			(f.Ability == "kind_xp_pct") != (f.Kind != "") || (f.Kind != "" && !slices.Contains(MixKinds[:], f.Kind)) {
			return fmt.Errorf("classes.yaml: 終職 %q 的 id／name／spec／ability／per_level／kind 不正確或重複", f.ID)
		}
		ids[f.ID], specs[f.Spec] = true, true
	}
	for _, c := range g.Classes {
		for _, s := range c.Specs {
			if !specs[s.ID] {
				return fmt.Errorf("classes.yaml: 特化 %q 沒有對應的終職", s.ID)
			}
			delete(specs, s.ID)
		}
	}
	for s := range specs {
		return fmt.Errorf("classes.yaml: 終職引用了不存在的特化 %q", s)
	}
	return nil
}

// FinalForSpec 回傳特化對應的終職；沒有時回傳 false。
func (p *Pack) FinalForSpec(spec string) (Final, bool) {
	for _, f := range p.Growth.Finals {
		if f.Spec == spec {
			return f, true
		}
	}
	return Final{}, false
}

// FinalByID 找出終職；找不到回傳 false。
func (p *Pack) FinalByID(id string) (Final, bool) {
	for _, f := range p.Growth.Finals {
		if f.ID == id {
			return f, true
		}
	}
	return Final{}, false
}
