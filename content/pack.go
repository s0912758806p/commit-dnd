// Package content 載入嵌入在執行檔裡的 YAML 遊戲內容（content.Pack）。
//
// balance.yaml（數值）、species.yaml（物種與 ASCII 圖）、logs.yaml（冒險日誌模板）
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
)

// Files 是 Pack 的原始 YAML；Parse 的輸入。
type Files struct {
	Balance, Species, Logs []byte
}

type Pack struct {
	Balance Balance
	Species []Species
	Logs    Logs
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
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Art  string `yaml:"art"`
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
	for _, b := range [][]byte{f.Balance, f.Species, f.Logs} {
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
		if s.ID == "" || s.Name == "" || seen[s.ID] {
			return fmt.Errorf("species.yaml: id 與 name 不可為空、id 不可重複（%q）", s.ID)
		}
		seen[s.ID] = true
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

// SpeciesByID 找出物種；找不到回傳 false。
func (p *Pack) SpeciesByID(id string) (Species, bool) {
	for _, s := range p.Species {
		if s.ID == id {
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
	return Parse(Files{Balance: balanceYAML, Species: speciesYAML, Logs: logsYAML})
})

// Default 回傳嵌入的正式內容；只解析一次。
func Default() (*Pack, error) { return defaultPack() }

// Embedded 回傳嵌入的原始 YAML，給測試改寫其中一個檔案用。
func Embedded() Files {
	return Files{Balance: balanceYAML, Species: speciesYAML, Logs: logsYAML}
}
