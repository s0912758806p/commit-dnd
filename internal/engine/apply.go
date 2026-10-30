// Package engine 是遊戲規則本身：Apply(state, event, content) → 新狀態、日誌、動畫提示。
//
// Apply 是純函式：不讀時鐘、不碰檔案、不用全域亂數，同樣輸入永遠同樣輸出，
// 這是重播、模擬與 Rebuild 的基礎。每次 Apply 先依 e.At − s.UpdatedAt 推進時間衰減
// （Δ < 0 時視為 0、UpdatedAt 只前進），再依 e.Type 分派。未知的 Type 只推進時間。
//
// 時間衰減必須「路徑無關」：一次推進 Δ1+Δ2 與分兩次推進結果相同（見 apply_test.go），
// 因為 status／statusline 套用的 tick 不會寫入 events.jsonl。
package engine

import (
	"fmt"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

type Result struct {
	State State
	Logs  []LogLine
	Fx    []Effect
}

func Apply(s State, e event.Event, c *content.Pack) Result {
	if s.V == 0 {
		s = initial(c)
	}
	s.ContentHash = c.Hash
	s = advance(s, e.At, c)
	if e.Seq > s.LastSeq {
		s.LastSeq = e.Seq
	}

	r := Result{State: s}
	switch e.Type {
	case "pet":
		r = pet(r, e, c)
	}
	return r
}

func initial(c *content.Pack) State {
	return State{V: StateVersion, Pet: Pet{Mood: c.Balance.Mood.Start}}
}

// advance 依經過時間扣心情。未滿一個週期的時間留在 MoodCarry，所以切分方式不影響結果。
func advance(s State, at time.Time, c *content.Pack) State {
	var elapsed time.Duration
	if !s.UpdatedAt.IsZero() && at.After(s.UpdatedAt) {
		elapsed = at.Sub(s.UpdatedAt)
	}
	if s.UpdatedAt.IsZero() || at.After(s.UpdatedAt) {
		s.UpdatedAt = at
	}

	m := c.Balance.Mood
	period := time.Duration(m.DecayEveryMinutes) * time.Minute
	total := s.Pet.MoodCarry + elapsed
	steps := int64(total / period)
	s.Pet.MoodCarry = total % period
	if steps > int64(s.Pet.Mood) { // 已足以歸零，避免乘法溢位
		steps = int64(s.Pet.Mood)
	}
	s.Pet.Mood = max(0, s.Pet.Mood-int(steps)*m.DecayAmount)
	return s
}

func pet(r Result, e event.Event, c *content.Pack) Result {
	m := c.Balance.Mood
	before := r.State.Pet.Mood
	r.State.Pet.Mood = min(m.Max, before+m.PerPet)
	r.State.Pet.Pets++
	r.Logs = append(r.Logs, LogLine{At: e.At, Text: fmt.Sprintf("你摸了摸寵物，心情 %d → %d。", before, r.State.Pet.Mood)})
	r.Fx = append(r.Fx, EffectHappy)
	return r
}
