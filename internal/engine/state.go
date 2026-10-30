package engine

import "time"

// StateVersion 是 state.json 的格式版本。state.json 只是快取，版本不符時 store 會從 events.jsonl 重建。
const StateVersion = 2

// State 是遊戲的完整狀態。所有數值一律用整數，確保 JSON 往返與跨平台重播結果完全相同。
// State 必須保持可比較（不含 map／slice），測試直接用 == 比對。
type State struct {
	V           int       `json:"v"` // 0 表示尚未有任何事件
	LastSeq     int64     `json:"last_seq"`
	UpdatedAt   time.Time `json:"updated_at"` // 只會前進，不會因時間倒流的事件而後退
	ContentHash string    `json:"content_hash"`
	// LastCommit 是最後一筆計分的 commit hash；同一個 hash 再來（hook 重跑、重複安裝）不重複計分。
	LastCommit string `json:"last_commit"`
	Pet        Pet    `json:"pet"`
}

type Pet struct {
	Species string `json:"species"` // 空字串表示還沒孵
	Name    string `json:"name"`
	Level   int    `json:"level"`
	XP      int    `json:"xp"` // 本級已累積的 XP，升級時扣掉該級成本
	HP      int    `json:"hp"`
	Satiety int    `json:"satiety"` // 千分位
	Mood    int    `json:"mood"`    // 千分位
	Pets    int    `json:"pets"`    // 被摸的次數
	Commits int    `json:"commits"`

	// 每日補給：SupplyDay 是 commit 當地日期（YYYY-MM-DD），SupplyUsed 是當天已用掉的加倍次數。
	SupplyDay  string `json:"supply_day"`
	SupplyUsed int    `json:"supply_used"`

	// *Carry 是尚未滿一個週期的累積時間；保留它才能讓時間效果與切分方式無關。
	HPCarry      time.Duration `json:"hp_carry_ns"`
	SatietyCarry time.Duration `json:"satiety_carry_ns"`
	MoodCarry    time.Duration `json:"mood_carry_ns"`
}

// Hatched 回報是否已經孵出寵物。
func (p Pet) Hatched() bool { return p.Species != "" }

type LogLine struct {
	At   time.Time
	Text string
}

// Effect 是給顯示層的動畫提示；顯示層只看它，不自己做遊戲判斷。
type Effect string

const (
	EffectHatch   Effect = "hatch"
	EffectHappy   Effect = "happy"
	EffectLevelUp Effect = "levelup"
	EffectHungry  Effect = "hungry"
	EffectSupply  Effect = "supply"
)
