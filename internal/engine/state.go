package engine

import "time"

// StateVersion 是 state.json 的格式版本。state.json 只是快取，版本不符時 store 會從 events.jsonl 重建。
const StateVersion = 5

// State 是遊戲的完整狀態。所有數值一律用整數，確保 JSON 往返與跨平台重播結果完全相同。
// State 必須保持可比較（不含 map／slice），測試直接用 == 比對。
type State struct {
	V           int       `json:"v"` // 0 表示尚未有任何事件
	LastSeq     int64     `json:"last_seq"`
	UpdatedAt   time.Time `json:"updated_at"` // 只會前進，不會因時間倒流的事件而後退
	ContentHash string    `json:"content_hash"`
	// LastCommit 是最後一筆計分的 commit hash；同一個 hash 再來（hook 重跑、重複安裝）不重複計分。
	LastCommit string  `json:"last_commit"`
	Pet        Pet     `json:"pet"`
	AI         AI      `json:"ai"`
	Dungeon    Dungeon `json:"dungeon"`
}

// BagSize 是未鑑定掉落物的背包容量；滿了之後新的掉落物會被丟掉。
const BagSize = 10

// Dungeon 是無盡地下城的進度。
type Dungeon struct {
	Floor   int     `json:"floor"`   // 目前樓層；0 表示還在入口
	Deepest int     `json:"deepest"` // 到過最深的樓層
	Warp    int     `json:"warp"`    // 已解鎖最深的傳送點（打倒過 Boss 的樓層）；0 表示沒有
	Downed  bool    `json:"downed"`  // 倒地中：AI 回合不再推進，等 commit 救援
	Monster Monster `json:"monster"` // 這層的怪物；ID 為空表示沒有（下個 AI 回合往下探）
	Weapon  Item    `json:"weapon"`  // 裝備中的武器；Name 為空表示沒有
	Armor   Item    `json:"armor"`
	// Bag 是未鑑定的掉落物，前 BagN 個有效（舊到新）。commit 鑑定最舊的一件；倒地時全部遺失。
	Bag   [BagSize]Drop `json:"bag"`
	BagN  int           `json:"bag_n"`
	Kills int           `json:"kills"`
	Downs int           `json:"downs"`
	// LastActive 是最後一次 commit 或 AI 活動的時間；之後閒置超過 camp_after_hours 就紮營回滿 HP。
	LastActive time.Time `json:"last_active"`
}

type Monster struct {
	ID    string `json:"id"` // content.Monster 的 id；名稱、屬性、是否 Boss 查內容
	HP    int    `json:"hp"`
	MaxHP int    `json:"max_hp"`
	Atk   int    `json:"atk"`
}

// Item 是鑑定後的裝備。Kind 為 "weapon" 或 "armor"。
type Item struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Element string `json:"element"`
	Power   int    `json:"power"`
}

// Drop 是未鑑定的掉落物：鑑定時用 Seed 決定種類、屬性、名稱與強度，重播結果不變。
type Drop struct {
	Floor int    `json:"floor"`
	Seed  uint64 `json:"seed"`
}

// AI 是 Claude Code／Codex 的活動統計。迭代 2 只計數；迭代 3 起 AI 回合會推進地下城。
type AI struct {
	Turns    int `json:"turns"`     // 累計 AI 回合（Claude 的 Stop）
	ToolUses int `json:"tool_uses"` // 累計工具呼叫（Claude 的 PostToolUse）
	// TodayTurns 是 Day（當地日期 YYYY-MM-DD）當天的 AI 回合數。
	Day        string `json:"day"`
	TodayTurns int    `json:"today_turns"`
}

// TurnsOn 回傳某個當地日期的 AI 回合數；不是記錄中的那天時為 0。
func (a AI) TurnsOn(day string) int {
	if day != a.Day {
		return 0
	}
	return a.TodayTurns
}

type Pet struct {
	Species string `json:"species"` // 空字串表示還沒孵
	Name    string `json:"name"`
	Class   string `json:"class"` // 職業；空字串表示尚未轉職（迭代 5 實作轉職）
	Stage   int    `json:"stage"` // 進化階段，孵出時為 1（迭代 5 實作進化）
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
	EffectDescend Effect = "descend"
	EffectKill    Effect = "kill"
	EffectBoss    Effect = "boss"
	EffectDowned  Effect = "downed"
	EffectRescue  Effect = "rescue"
	EffectCamp    Effect = "camp"
)
