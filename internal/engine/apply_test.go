package engine

import (
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// 小型假內容：每 10 分鐘心情 -1，方便手算。
var testPack = &content.Pack{
	Hash: "test",
	Balance: content.Balance{Mood: content.Mood{
		Start: 100, Max: 120, PerPet: 15, DecayEveryMinutes: 10, DecayAmount: 1,
	}},
}

var t0 = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

func ev(typ string, seq int64, at time.Time) event.Event {
	return event.Event{V: event.Version, Seq: seq, At: at, Source: event.SourceUser, Type: typ}
}

func tick(at time.Time) event.Event {
	return event.Event{At: at, Source: event.SourceTick, Type: "tick"}
}

func TestApply(t *testing.T) {
	started := Apply(State{}, ev("pet", 1, t0), testPack).State // Mood 115

	cases := []struct {
		name     string
		s        State
		e        event.Event
		mood     int
		pets     int
		lastSeq  int64
		updated  time.Time
		wantLogs int
	}{
		{"第一個事件初始化並套用", State{}, ev("pet", 1, t0), 115, 1, 1, t0, 1},
		{"心情不超過上限", started, ev("pet", 2, t0), 120, 2, 2, t0, 1},
		{"經過 25 分鐘扣 2", started, tick(t0.Add(25 * time.Minute)), 113, 1, 1, t0.Add(25 * time.Minute), 0},
		{"心情夾在 0", started, tick(t0.Add(365 * 24 * time.Hour)), 0, 1, 1, t0.Add(365 * 24 * time.Hour), 0},
		{"時間倒流不衰減也不倒退 UpdatedAt", started, ev("pet", 2, t0.Add(-time.Hour)), 120, 2, 2, t0, 1},
		{"未知類型只推進時間", started, ev("nope", 2, t0.Add(10*time.Minute)), 114, 1, 2, t0.Add(10 * time.Minute), 0},
		{"tick 不改 LastSeq", started, tick(t0), 115, 1, 1, t0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Apply(tc.s, tc.e, testPack)
			s := r.State
			if s.Pet.Mood != tc.mood || s.Pet.Pets != tc.pets || s.LastSeq != tc.lastSeq || !s.UpdatedAt.Equal(tc.updated) {
				t.Fatalf("got mood=%d pets=%d seq=%d at=%v", s.Pet.Mood, s.Pet.Pets, s.LastSeq, s.UpdatedAt)
			}
			if len(r.Logs) != tc.wantLogs {
				t.Fatalf("logs = %v", r.Logs)
			}
			if s.V != StateVersion || s.ContentHash != "test" {
				t.Fatalf("V=%d hash=%q", s.V, s.ContentHash)
			}
		})
	}
}

// 時間衰減必須與切分方式無關：status 只在記憶體套用 tick、不落地，
// 所以「一次算 Δ1+Δ2」與「分兩次算」必須得到同樣狀態，Rebuild 才會和實際一致。
func TestDecayIsPathIndependent(t *testing.T) {
	base := Apply(State{}, ev("pet", 1, t0), testPack).State
	splits := []time.Duration{0, time.Second, 7 * time.Minute, 9*time.Minute + 59*time.Second, 10 * time.Minute, 3*time.Hour + 1, 40 * time.Hour}
	for _, total := range []time.Duration{0, 9 * time.Minute, 95 * time.Minute, 50 * time.Hour} {
		once := Apply(base, tick(t0.Add(total)), testPack).State
		for _, d1 := range splits {
			if d1 > total {
				continue
			}
			mid := Apply(base, tick(t0.Add(d1)), testPack).State
			twice := Apply(mid, tick(t0.Add(total)), testPack).State
			if once != twice {
				t.Fatalf("total=%v d1=%v: 一次=%+v 兩次=%+v", total, d1, once.Pet, twice.Pet)
			}
		}
	}
}

func TestApplyIsDeterministic(t *testing.T) {
	events := []event.Event{ev("pet", 1, t0), ev("pet", 2, t0.Add(time.Hour)), ev("pet", 3, t0.Add(90*time.Minute))}
	run := func() State {
		var s State
		for _, e := range events {
			s = Apply(s, e, testPack).State
		}
		return s
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("%+v != %+v", a, b)
	}
}
