// Package content 載入嵌入在執行檔裡的 YAML 遊戲內容（content.Pack）。
//
// balance.yaml（數值）、species.yaml（物種）、logs.yaml（冒險日誌模板）、
// monsters.yaml／floors.yaml／items.yaml（地下城）
// 以 go:embed 編進執行檔，所以調整內容只要重新編譯。XP 曲線在載入時預先算成整數表，
// 引擎與 State 內不出現浮點。Pack.Hash 是三個檔案的雜湊，引擎把它寫進 State.ContentHash，
// 用來偵測「用不同內容重播」造成的差異。測試可以直接組一個 &Pack{...} 注入，不必經過 YAML。
package content

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"sync"

	"go.yaml.in/yaml/v3"
)

var (
	//go:embed balance.yaml
	balanceYAML []byte
	//go:embed species.yaml
	speciesYAML []byte
	//go:embed logs.yaml
	logsYAML []byte
	//go:embed monsters.yaml
	monstersYAML []byte
	//go:embed floors.yaml
	floorsYAML []byte
	//go:embed items.yaml
	itemsYAML []byte
)

// Files 是 Pack 的原始 YAML；Parse 的輸入。
type Files struct {
	Balance, Species, Logs, Monsters, Floors, Items []byte
}

type Pack struct {
	Balance  Balance
	Species  []Species
	Logs     Logs
	Monsters []Monster
	Floors   Floors
	Items    Items
	// XPTable[n] 是從 Lv n 升到 n+1 需要的 XP；索引 0 不使用，長度為 max_level。
	XPTable []int
	Hash    string // sha256 前 12 碼
}

type Balance struct {
	XP      XP      `yaml:"xp"`
	Commit  Commit  `yaml:"commit"`
	Supply  Supply  `yaml:"supply"`
	HP      HP      `yaml:"hp"`
	Satiety Satiety `yaml:"satiety"`
	Mood    Mood    `yaml:"mood"`
	Dungeon Dungeon `yaml:"dungeon"`
}

type Dungeon struct {
	GrowthPerFloorPct      int      `yaml:"growth_per_floor_pct"`
	BossEvery              int      `yaml:"boss_every"`
	BossMultiplierPct      int      `yaml:"boss_multiplier_pct"`
	ElementAdvantagePct    int      `yaml:"element_advantage_pct"`
	ElementDisadvantagePct int      `yaml:"element_disadvantage_pct"`
	PetAtkBase             int      `yaml:"pet_atk_base"`
	PetAtkPerLevel         int      `yaml:"pet_atk_per_level"`
	PetDefPerLevel         int      `yaml:"pet_def_per_level"`
	RescueHPPct            int      `yaml:"rescue_hp_pct"`
	CampAfterHours         int      `yaml:"camp_after_hours"`
	ItemPowerBase          int      `yaml:"item_power_base"`
	ItemPowerPerFloors     int      `yaml:"item_power_per_floors"`
	AttackTools            []string `yaml:"attack_tools"`
}

// Elements 是三種屬性；Beats[a] 是 a 剋制的屬性（火剋草、草剋水、水剋火）。
var (
	Elements = []string{"fire", "water", "grass"}
	Beats    = map[string]string{"fire": "grass", "grass": "water", "water": "fire"}
)

// ElementName 是屬性的中文名稱。
var ElementName = map[string]string{"fire": "火", "water": "水", "grass": "草"}

type Monster struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Element string `yaml:"element"`
	HP      int    `yaml:"hp"`
	Atk     int    `yaml:"atk"`
	Boss    bool   `yaml:"boss"`
}

type Floors struct {
	Tiers []struct {
		From     int      `yaml:"from"`
		Monsters []string `yaml:"monsters"`
	} `yaml:"tiers"`
	Bosses []string `yaml:"bosses"`
}

// Items 是裝備名稱：Weapon／Armor[屬性] = 名稱清單。
type Items struct {
	Weapon map[string][]string `yaml:"weapon"`
	Armor  map[string][]string `yaml:"armor"`
}

type XP struct {
	CurveBase float64 `yaml:"curve_base"`
	CurveExp  float64 `yaml:"curve_exp"`
	MaxLevel  int     `yaml:"max_level"`
}

type Commit struct {
	Base            map[string]int `yaml:"base"` // 依 commit 類型；必須有 other
	LineBonusPerBit int            `yaml:"line_bonus_per_bit"`
	LineBonusMax    int            `yaml:"line_bonus_max"`
	Feed            int            `yaml:"feed"`
	Heal            int            `yaml:"heal"`
}

type Supply struct {
	CommitsPerDay int `yaml:"commits_per_day"`
	Multiplier    int `yaml:"multiplier"`
}

type HP struct {
	Base              int `yaml:"base"`
	PerLevel          int `yaml:"per_level"`
	RegenEveryMinutes int `yaml:"regen_every_minutes"`
	RegenAmount       int `yaml:"regen_amount"`
}

// Gauge 是隨時間下降的千分位量表（飽足、心情）。
type Gauge struct {
	Start             int `yaml:"start"`
	Max               int `yaml:"max"`
	DecayEveryMinutes int `yaml:"decay_every_minutes"`
	DecayAmount       int `yaml:"decay_amount"`
}

type Satiety struct {
	Gauge       `yaml:",inline"`
	HungryBelow int `yaml:"hungry_below"`
}

type Mood struct {
	Gauge  `yaml:",inline"`
	PerPet int `yaml:"per_pet"`
}

type Species struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Aliases []string `yaml:"aliases"` // 舊 id；重播舊事件時對應到這個物種
}

type Logs struct {
	Hatch          string              `yaml:"hatch"`
	AlreadyHatched string              `yaml:"already_hatched"`
	Levelup        string              `yaml:"levelup"`
	Supply         string              `yaml:"supply"`
	Hungry         string              `yaml:"hungry"`
	Pet            string              `yaml:"pet"`
	NoPet          string              `yaml:"no_pet"`
	Commit         map[string][]string `yaml:"commit"` // 依 commit 類型；必須有 other
}

// Parse 解析並驗證內容；未知欄位視為錯誤，避免打錯字默默失效。
func Parse(f Files) (*Pack, error) {
	var p Pack
	for _, d := range []struct {
		name string
		src  []byte
		dst  any
	}{
		{"balance.yaml", f.Balance, &p.Balance},
		{"species.yaml", f.Species, &p.Species},
		{"logs.yaml", f.Logs, &p.Logs},
		{"monsters.yaml", f.Monsters, &p.Monsters},
		{"floors.yaml", f.Floors, &p.Floors},
		{"items.yaml", f.Items, &p.Items},
	} {
		dec := yaml.NewDecoder(bytes.NewReader(d.src))
		dec.KnownFields(true)
		if err := dec.Decode(d.dst); err != nil {
			return nil, fmt.Errorf("%s: %w", d.name, err)
		}
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	x := p.Balance.XP
	p.XPTable = XPTable(x.CurveBase, x.CurveExp, x.MaxLevel)

	h := sha256.New()
	for _, b := range [][]byte{f.Balance, f.Species, f.Logs, f.Monsters, f.Floors, f.Items} {
		h.Write(b)
		h.Write([]byte{0})
	}
	p.Hash = hex.EncodeToString(h.Sum(nil))[:12]
	return &p, nil
}

// XPTable 回傳 table[n] = round(base × n^exp)，n = 1..maxLevel-1。
func XPTable(base, exp float64, maxLevel int) []int {
	t := make([]int, maxLevel)
	for n := 1; n < maxLevel; n++ {
		t[n] = int(math.Round(base * math.Pow(float64(n), exp)))
	}
	return t
}

func (p *Pack) validate() error {
	b := p.Balance
	switch {
	case b.XP.CurveBase <= 0 || b.XP.CurveExp <= 0 || b.XP.MaxLevel < 2:
		return fmt.Errorf("balance.yaml: xp 的曲線參數必須 > 0，max_level ≥ 2")
	case b.Commit.Base["other"] <= 0:
		return fmt.Errorf("balance.yaml: commit.base 必須有 other 且 > 0")
	case b.Supply.Multiplier < 1 || b.Supply.CommitsPerDay < 0:
		return fmt.Errorf("balance.yaml: supply.multiplier ≥ 1、commits_per_day ≥ 0")
	case b.HP.Base <= 0 || b.HP.RegenEveryMinutes <= 0:
		return fmt.Errorf("balance.yaml: hp.base 與 hp.regen_every_minutes 必須 > 0")
	}
	for name, g := range map[string]Gauge{"satiety": b.Satiety.Gauge, "mood": b.Mood.Gauge} {
		if g.Max <= 0 || g.Start < 0 || g.Start > g.Max || g.DecayEveryMinutes <= 0 || g.DecayAmount < 0 {
			return fmt.Errorf("balance.yaml: %s 需 max > 0、0 ≤ start ≤ max、decay_every_minutes > 0、decay_amount ≥ 0", name)
		}
	}

	if len(p.Species) == 0 {
		return fmt.Errorf("species.yaml: 至少要有一個物種")
	}
	seen := map[string]bool{}
	for _, s := range p.Species {
		if s.ID == "" || s.Name == "" {
			return fmt.Errorf("species.yaml: id 與 name 不可為空（%q）", s.ID)
		}
		for _, id := range append([]string{s.ID}, s.Aliases...) {
			if id == "" || seen[id] {
				return fmt.Errorf("species.yaml: id 與 aliases 不可為空或重複（%q）", id)
			}
			seen[id] = true
		}
	}

	if err := p.validateDungeon(); err != nil {
		return err
	}

	if len(p.Logs.Commit["other"]) == 0 {
		return fmt.Errorf("logs.yaml: commit 必須有 other")
	}
	for kind, lines := range p.Logs.Commit {
		if len(lines) == 0 {
			return fmt.Errorf("logs.yaml: commit.%s 至少要一句", kind)
		}
	}
	return nil
}

// SpeciesByID 用 id 或舊 id（aliases）找出物種；找不到回傳 false。
func (p *Pack) SpeciesByID(id string) (Species, bool) {
	for _, s := range p.Species {
		if s.ID == id || slices.Contains(s.Aliases, id) {
			return s, true
		}
	}
	return Species{}, false
}

// Cost 回傳從 Lv level 升到下一級需要的 XP；已達上限時回傳 0。
func (p *Pack) Cost(level int) int {
	if level < 1 || level >= len(p.XPTable) {
		return 0
	}
	return p.XPTable[level]
}

// MaxHP 回傳某等級的 HP 上限。
func (p *Pack) MaxHP(level int) int {
	return p.Balance.HP.Base + p.Balance.HP.PerLevel*(level-1)
}

var defaultPack = sync.OnceValues(func() (*Pack, error) {
	return Parse(Embedded())
})

// Default 回傳嵌入的正式內容；只解析一次。
func Default() (*Pack, error) { return defaultPack() }

// Embedded 回傳嵌入的原始 YAML，給測試改寫其中一個檔案用。
func Embedded() Files {
	return Files{Balance: balanceYAML, Species: speciesYAML, Logs: logsYAML, Monsters: monstersYAML, Floors: floorsYAML, Items: itemsYAML}
}

func (p *Pack) validateDungeon() error {
	d := p.Balance.Dungeon
	if d.BossEvery < 1 || d.GrowthPerFloorPct < 0 || d.BossMultiplierPct < 100 || d.RescueHPPct < 1 || d.RescueHPPct > 100 ||
		d.CampAfterHours < 1 || d.ItemPowerPerFloors < 1 || d.ElementAdvantagePct < 100 || d.ElementDisadvantagePct < 1 {
		return fmt.Errorf("balance.yaml: dungeon 的數值超出範圍")
	}
	seen := map[string]bool{}
	for _, m := range p.Monsters {
		if m.ID == "" || m.Name == "" || seen[m.ID] || Beats[m.Element] == "" || m.HP < 1 || m.Atk < 1 {
			return fmt.Errorf("monsters.yaml: %q 的 id／name／element／hp／atk 不正確或 id 重複", m.ID)
		}
		seen[m.ID] = true
	}
	need := func(ids []string, boss bool, where string) error {
		if len(ids) == 0 {
			return fmt.Errorf("floors.yaml: %s 不可為空", where)
		}
		for _, id := range ids {
			m, ok := p.MonsterByID(id)
			if !ok || m.Boss != boss {
				return fmt.Errorf("floors.yaml: %s 的 %q 不存在或 boss 標記不符", where, id)
			}
		}
		return nil
	}
	if len(p.Floors.Tiers) == 0 || p.Floors.Tiers[0].From != 1 {
		return fmt.Errorf("floors.yaml: tiers 必須從 from: 1 開始")
	}
	for i, t := range p.Floors.Tiers {
		if i > 0 && t.From <= p.Floors.Tiers[i-1].From {
			return fmt.Errorf("floors.yaml: tiers 的 from 必須遞增")
		}
		if err := need(t.Monsters, false, fmt.Sprintf("tiers[%d]", i)); err != nil {
			return err
		}
	}
	if err := need(p.Floors.Bosses, true, "bosses"); err != nil {
		return err
	}
	for _, e := range Elements {
		if len(p.Items.Weapon[e]) == 0 || len(p.Items.Armor[e]) == 0 {
			return fmt.Errorf("items.yaml: weapon 與 armor 都要有 %s 屬性的名稱", e)
		}
	}
	return nil
}

// MonsterByID 找出怪物；找不到回傳 false。
func (p *Pack) MonsterByID(id string) (Monster, bool) {
	for _, m := range p.Monsters {
		if m.ID == id {
			return m, true
		}
	}
	return Monster{}, false
}

// TierMonsters 回傳某層可能出現的一般怪物 id。
func (p *Pack) TierMonsters(floor int) []string {
	ids := p.Floors.Tiers[0].Monsters
	for _, t := range p.Floors.Tiers {
		if t.From <= floor {
			ids = t.Monsters
		}
	}
	return ids
}

// BossFor 回傳 Boss 層的 Boss id；不是 Boss 層時回傳空字串。
func (p *Pack) BossFor(floor int) string {
	every := p.Balance.Dungeon.BossEvery
	if floor < every || floor%every != 0 {
		return ""
	}
	return p.Floors.Bosses[(floor/every-1)%len(p.Floors.Bosses)]
}

// IsAttackTool 回報這個工具呼叫是否算寵物攻擊一次。
func (p *Pack) IsAttackTool(tool string) bool {
	return slices.Contains(p.Balance.Dungeon.AttackTools, tool)
}
