package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// 小型假內容，數字方便手算。XP：Lv1→2 = 18、Lv2→3 = 44、Lv3→4 = 75。
var testPack = &content.Pack{
	Hash: "test",
	Balance: content.Balance{
		Commit: content.Commit{
			Base:            map[string]int{"feat": 30, "other": 10},
			LineBonusPerBit: 2, LineBonusMax: 20, Feed: 150, Heal: 5,
		},
		Supply:  content.Supply{CommitsPerDay: 3, Multiplier: 2},
		HP:      content.HP{Base: 20, PerLevel: 5, RegenEveryMinutes: 30, RegenAmount: 1},
		Satiety: content.Satiety{Gauge: content.Gauge{Start: 800, Max: 1000, DecayEveryMinutes: 60, DecayAmount: 10}, HungryBelow: 200},
		Mood:    content.Mood{Gauge: content.Gauge{Start: 500, Max: 1000, DecayEveryMinutes: 60, DecayAmount: 10}, PerPet: 50},
	},
	Species: []content.Species{{ID: "slime", Name: "史萊姆"}},
	Logs: content.Logs{
		Hatch: "孵出{species}「{name}」", AlreadyHatched: "已有{name}", Levelup: "{name} Lv{level}",
		Supply: "補給", Hungry: "餓", Pet: "摸{name}", NoPet: "沒有寵物",
		Commit: map[string][]string{"feat": {"{name} 新功能 +{xp}"}, "other": {"{name} 搬了 {lines} 行 +{xp}"}},
	},
	XPTable: content.XPTable(18, 1.3, 10),
}

var (
	cst = time.FixedZone("CST", 8*3600)
	t0  = time.Date(2026, 10, 30, 9, 0, 0, 0, cst)
)

func ev(typ string, at time.Time) event.Event {
	return event.Event{V: event.Version, At: at, Source: event.SourceUser, Type: typ}
}

func hatchEv(at time.Time, species, name string) event.Event {
	e := ev("hatch", at)
	e.Args = map[string]string{"species": species, "name": name}
	return e
}

func commitEv(at time.Time, kind string, lines int, hash string) event.Event {
	e := ev("commit", at)
	e.Source = event.SourceGit
	e.Commit = &event.CommitInfo{Kind: kind, Added: lines, Hash: hash}
	return e
}

func tick(at time.Time) event.Event {
	return event.Event{At: at, Source: event.SourceTick, Type: "tick"}
}

func run(s State, events ...event.Event) Result {
	var r Result
	for _, e := range events {
		r = Apply(s, e, testPack)
		s = r.State
	}
	return r
}

var hatched = run(State{}, hatchEv(t0, "slime", "小橘")).State

func texts(r Result) string {
	var b []string
	for _, l := range r.Logs {
		b = append(b, l.Text)
	}
	return strings.Join(b, " | ")
}

func TestHatch(t *testing.T) {
	cases := []struct {
		name string
		s    State
		e    event.Event
		pet  Pet
		logs string
	}{
		{"孵出寵物", State{}, hatchEv(t0, "slime", "小橘"),
			Pet{Species: "slime", Name: "小橘", Level: 1, HP: 20, Satiety: 800, Mood: 500}, "孵出史萊姆「小橘」"},
		{"沒取名就用物種名", State{}, hatchEv(t0, "slime", ""),
			Pet{Species: "slime", Name: "史萊姆", Level: 1, HP: 20, Satiety: 800, Mood: 500}, "孵出史萊姆「史萊姆」"},
		{"未知物種不孵", State{}, hatchEv(t0, "unicorn", "x"), Pet{}, ""},
		{"已經有寵物", hatched, hatchEv(t0, "slime", "阿呆"), hatched.Pet, "已有小橘"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Apply(tc.s, tc.e, testPack)
			if r.State.Pet != tc.pet || texts(r) != tc.logs {
				t.Fatalf("pet=%+v logs=%q", r.State.Pet, texts(r))
			}
		})
	}
}

func TestCommit(t *testing.T) {
	cases := []struct {
		name   string
		s      State
		events []event.Event
		level  int
		xp     int
		logs   string // 最後一個事件的日誌
	}{
		{"孵蛋前的 commit 只留日誌", State{}, []event.Event{commitEv(t0, "feat", 0, "a")}, 0, 0, "沒有寵物"},
		// 30 × 2（補給）= 60 → 升到 Lv2 剩 42
		{"第一筆 commit 補給加倍並升級", hatched, []event.Event{commitEv(t0, "feat", 0, "a")}, 2, 42, "補給 | 小橘 新功能 +60 | 小橘 Lv2"},
		// 不認得的類型算 other：10 + 行數加成 2×bits.Len(100)=14 → 24 × 2 = 48 → Lv2 剩 30
		{"未知類型與行數加成", hatched, []event.Event{commitEv(t0, "wip", 100, "a")}, 2, 30, "補給 | 小橘 搬了 100 行 +48 | 小橘 Lv2"},
		// 行數加成上限 20：10 + 20 = 30 × 2 = 60
		{"行數加成有上限", hatched, []event.Event{commitEv(t0, "other", 1_000_000, "a")}, 2, 42, "補給 | 小橘 搬了 1000000 行 +60 | 小橘 Lv2"},
		// 一次升兩級：feat + 1000 行 = 30 + 20 = 50 × 2 = 100 → -18 → -44 → Lv3 剩 38
		{"一次升兩級", hatched, []event.Event{commitEv(t0, "feat", 1000, "a")}, 3, 38, "補給 | 小橘 新功能 +100 | 小橘 Lv2 | 小橘 Lv3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(tc.s, tc.events...)
			p := r.State.Pet
			if p.Level != tc.level || p.XP != tc.xp || texts(r) != tc.logs {
				t.Fatalf("level=%d xp=%d logs=%q", p.Level, p.XP, texts(r))
			}
		})
	}
}

func TestSameCommitCountsOnce(t *testing.T) {
	once := run(hatched, commitEv(t0, "feat", 0, "abc"))
	twice := run(once.State, commitEv(t0, "feat", 0, "abc"))
	if twice.State.Pet != once.State.Pet || len(twice.Logs) != 0 {
		t.Fatalf("同一個 hash 不可重複計分: %+v %q", twice.State.Pet, texts(twice))
	}
	if r := run(once.State, commitEv(t0, "feat", 0, "def")); r.State.Pet.Commits != 2 {
		t.Fatal("不同 hash 應該計分")
	}
}

func TestLevelUpRestoresHP(t *testing.T) {
	s := hatched
	s.Pet.HP = 3
	p := run(s, commitEv(t0, "feat", 0, "a")).State.Pet
	if p.HP != testPack.MaxHP(2) {
		t.Fatalf("HP = %d, want %d", p.HP, testPack.MaxHP(2))
	}
}

func TestCommitFeedsAndHeals(t *testing.T) {
	s := hatched
	s.Pet.HP, s.Pet.Satiety, s.Pet.Level, s.Pet.XP = 10, 900, 5, 0 // Lv5 不會因這筆升級
	p := run(s, commitEv(t0, "other", 0, "a")).State.Pet
	if p.Satiety != 1000 || p.HP != 15 || p.Commits != 1 {
		t.Fatalf("satiety=%d hp=%d commits=%d", p.Satiety, p.HP, p.Commits)
	}
}

func TestDailySupply(t *testing.T) {
	s := hatched
	s.Pet.Level = 5 // 避免升級干擾 XP 計算
	var xps []int
	at := []time.Time{
		t0, t0.Add(time.Minute), t0.Add(2 * time.Minute), t0.Add(3 * time.Minute), // 同一天 4 筆
		time.Date(2026, 10, 31, 0, 30, 0, 0, cst), // 當地隔天（UTC 仍是 10/30）
	}
	for i, a := range at {
		before := s.Pet.XP
		s = run(s, commitEv(a, "other", 0, string(rune('a'+i)))).State
		xps = append(xps, s.Pet.XP-before)
	}
	want := []int{20, 20, 20, 10, 20}
	for i := range want {
		if xps[i] != want[i] {
			t.Fatalf("XP 增量 = %v, want %v", xps, want)
		}
	}
}

func TestHungryHalvesXP(t *testing.T) {
	s := hatched
	s.Pet.Level, s.Pet.Satiety, s.Pet.SupplyDay, s.Pet.SupplyUsed = 5, 100, "2026-10-30", 3
	r := run(s, commitEv(t0, "feat", 0, "a"))
	if r.State.Pet.XP != 15 || r.State.Pet.Satiety != 250 || texts(r) != "餓 | 小橘 新功能 +15" {
		t.Fatalf("xp=%d satiety=%d logs=%q", r.State.Pet.XP, r.State.Pet.Satiety, texts(r))
	}
}

func TestPet(t *testing.T) {
	if r := Apply(State{}, ev("pet", t0), testPack); texts(r) != "沒有寵物" || r.State.Pet.Pets != 0 {
		t.Fatalf("孵蛋前 pet: %+v %q", r.State.Pet, texts(r))
	}
	s := hatched
	s.Pet.Mood = 980
	r := Apply(s, ev("pet", t0), testPack)
	if r.State.Pet.Mood != 1000 || r.State.Pet.Pets != 1 || texts(r) != "摸小橘" {
		t.Fatalf("pet: %+v %q", r.State.Pet, texts(r))
	}
}

func TestTime(t *testing.T) {
	cases := []struct {
		name    string
		s       State
		e       event.Event
		satiety int
		updated time.Time
	}{
		{"經過 2.5 小時扣 2 次", hatched, tick(t0.Add(150 * time.Minute)), 780, t0.Add(150 * time.Minute)},
		{"夾在 0", hatched, tick(t0.Add(365 * 24 * time.Hour)), 0, t0.Add(365 * 24 * time.Hour)},
		{"時間倒流不衰減也不倒退", hatched, tick(t0.Add(-time.Hour)), 800, t0},
		{"孵蛋前不衰減只推進時間", State{}, tick(t0), 0, t0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Apply(tc.s, tc.e, testPack).State
			if s.Pet.Satiety != tc.satiety || !s.UpdatedAt.Equal(tc.updated) {
				t.Fatalf("satiety=%d at=%v", s.Pet.Satiety, s.UpdatedAt)
			}
		})
	}
}

func TestSeqAndMeta(t *testing.T) {
	e := ev("nope", t0)
	e.Seq = 7
	s := Apply(State{}, e, testPack).State
	if s.LastSeq != 7 || s.V != StateVersion || s.ContentHash != "test" {
		t.Fatalf("%+v", s)
	}
	if s = Apply(s, tick(t0), testPack).State; s.LastSeq != 7 {
		t.Fatal("tick 不可改 LastSeq")
	}
}

// 時間效果必須與切分方式無關：status 只在記憶體套用 tick、不落地，
// 所以「一次算 Δ1+Δ2」與「分兩次算」必須得到同樣狀態，Rebuild 才會和實際一致。
// 新增任何時間效果都要讓它在這裡被測到。
func TestTimeEffectsArePathIndependent(t *testing.T) {
	base := hatched
	base.Pet.HP = 1 // 讓回血可被觀察
	splits := []time.Duration{0, time.Second, 7 * time.Minute, 29*time.Minute + 59*time.Second, 30 * time.Minute, 59 * time.Minute, 3*time.Hour + 1, 40 * time.Hour}
	for _, total := range []time.Duration{0, 9 * time.Minute, 95 * time.Minute, 10 * time.Hour, 200 * time.Hour} {
		once := Apply(base, tick(t0.Add(total)), testPack).State
		for _, d1 := range splits {
			if d1 > total {
				continue
			}
			mid := Apply(base, tick(t0.Add(d1)), testPack).State
			twice := Apply(mid, tick(t0.Add(total)), testPack).State
			if once != twice {
				t.Fatalf("total=%v d1=%v:\n一次=%+v\n兩次=%+v", total, d1, once.Pet, twice.Pet)
			}
		}
	}
}

func TestMaxLevelCapsLevel(t *testing.T) {
	s := hatched
	s.Pet.Level = 9 // testPack 的 max_level 是 10
	for i := range 50 {
		s = run(s, commitEv(t0, "feat", 1000, string(rune('a'+i)))).State
	}
	if s.Pet.Level != 10 {
		t.Fatalf("level = %d", s.Pet.Level)
	}
}

func TestApplyIsDeterministic(t *testing.T) {
	events := []event.Event{hatchEv(t0, "slime", "小橘"), commitEv(t0.Add(time.Hour), "feat", 12, "abc"), ev("pet", t0.Add(90*time.Minute))}
	if a, b := run(State{}, events...), run(State{}, events...); a.State != b.State || texts(a) != texts(b) {
		t.Fatalf("%+v != %+v", a.State, b.State)
	}
}
