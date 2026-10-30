package engine

import "time"

// StateVersion 是 state.json 的格式版本。state.json 只是快取，格式變了就從 events.jsonl 重建。
const StateVersion = 1

// State 是遊戲的完整狀態。所有數值一律用整數，確保 JSON 往返與跨平台重播結果完全相同。
// State 必須保持可比較（不含 map／slice），測試直接用 == 比對。
type State struct {
	V           int       `json:"v"` // 0 表示尚未有任何事件
	LastSeq     int64     `json:"last_seq"`
	UpdatedAt   time.Time `json:"updated_at"` // 只會前進，不會因時間倒流的事件而後退
	ContentHash string    `json:"content_hash"`
	Pet         Pet       `json:"pet"`
}

type Pet struct {
	Mood int `json:"mood"` // 千分位
	Pets int `json:"pets"` // 被摸的次數
	// MoodCarry 是尚未滿一個衰減週期的累積時間；保留它才能讓衰減與切分方式無關。
	MoodCarry time.Duration `json:"mood_carry_ns"`
}

type LogLine struct {
	At   time.Time
	Text string
}

// Effect 是給顯示層的動畫提示；顯示層只看它，不自己做遊戲判斷。
type Effect string

const EffectHappy Effect = "happy"
