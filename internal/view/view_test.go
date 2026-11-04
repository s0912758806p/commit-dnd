package view

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

var update = flag.Bool("update", false, "用目前輸出覆寫 testdata/golden")

var now = time.Date(2026, 10, 30, 21, 0, 0, 0, time.FixedZone("CST", 8*3600))

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v（第一次請加 -update）", err)
	}
	if got != string(want) {
		t.Fatalf("%s 不符：\n--- got\n%s--- want\n%s", name, got, want)
	}
}

func TestStatus(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	pet := engine.Pet{
		Species: "human", Name: "小橘", Stage: 1, Level: 2, XP: 42, HP: 20, Satiety: 950, Mood: 500,
		SupplyDay: "2026-10-30", SupplyUsed: 1,
	}
	cases := map[string]engine.Pet{
		"status.txt":        pet,
		"status_hungry.txt": func() engine.Pet { p := pet; p.Species, p.Satiety, p.SupplyUsed = "undead", 120, 3; return p }(),
		"status_newday.txt": func() engine.Pet {
			p := pet
			p.Species, p.SupplyDay, p.SupplyUsed = "dragonkin", "2026-10-29", 3
			return p
		}(),
		"status_alias.txt": func() engine.Pet { p := pet; p.Species, p.Name = "knight", "GR騎士"; return p }(), // 舊 id 也要顯示為人族
		"status_maxlevel.txt": func() engine.Pet {
			p := pet
			p.Level, p.HP = c.Balance.XP.MaxLevel, c.MaxHP(c.Balance.XP.MaxLevel)
			return p
		}(),
		"status_egg.txt": {},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			golden(t, name, Status(engine.State{Pet: p}, c, now))
		})
	}
}

// 地城那一行：入口不顯示、最深比目前深時附註、倒地中附註。
func TestStatusDungeon(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	pet := engine.Pet{Species: "elf", Name: "小橘", Stage: 1, Level: 6, XP: 10, HP: 3, Satiety: 800, Mood: 600}
	got := Status(engine.State{Pet: pet, Dungeon: engine.Dungeon{Floor: 12, Deepest: 15, Warp: 10}}, c, now) +
		"---\n" + Status(engine.State{Pet: pet, Dungeon: engine.Dungeon{Floor: 15, Deepest: 15, Downed: true}}, c, now)
	golden(t, "status_dungeon.txt", got)
}

func TestDungeon(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	pet := engine.Pet{Species: "human", Name: "小橘", Stage: 1, Level: 6, HP: 30, Satiety: 800, Mood: 600}
	explore := engine.Dungeon{
		Floor: 12, Deepest: 15, Warp: 10, Kills: 23, Downs: 1,
		Monster: engine.Monster{ID: "spaghetti", HP: 19, MaxHP: 41, Atk: 7},
	}
	cases := map[string]engine.Dungeon{
		"dungeon_entrance.txt":      {},
		"dungeon_entrance_warp.txt": {Deepest: 15, Warp: 10, Kills: 23, Downs: 1},
		"dungeon_monster.txt":       explore,
		"dungeon_boss.txt": func() engine.Dungeon {
			d := explore
			d.Floor, d.Deepest = 20, 20
			d.Monster = engine.Monster{ID: c.BossFor(20), HP: 120, MaxHP: 120, Atk: 18}
			return d
		}(),
		"dungeon_cleared.txt": func() engine.Dungeon { d := explore; d.Monster = engine.Monster{}; return d }(),
		"dungeon_downed.txt": func() engine.Dungeon {
			d := explore
			d.Floor, d.Downed, d.Downs = 15, true, 2
			d.Monster = engine.Monster{ID: "flaky", HP: 33, MaxHP: 40, Atk: 12}
			return d
		}(),
		"dungeon_gear.txt": func() engine.Dungeon {
			d := explore
			d.Weapon = engine.Item{Kind: "weapon", Name: c.Items.Weapon["fire"][0], Element: "fire", Power: 7}
			d.Armor = engine.Item{Kind: "armor", Name: c.Items.Armor["grass"][1], Element: "grass", Power: 4}
			d.Bag[0], d.Bag[1], d.Bag[2] = engine.Drop{Floor: 11, Seed: 1}, engine.Drop{Floor: 12, Seed: 2}, engine.Drop{Floor: 12, Seed: 3}
			d.BagN = 3
			return d
		}(),
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			golden(t, name, Dungeon(engine.State{Pet: pet, Dungeon: d}, c))
		})
	}
	golden(t, "dungeon_egg.txt", Dungeon(engine.State{}, c))
}

func TestLog(t *testing.T) {
	lines := []engine.LogLine{
		{At: now.Add(-2 * time.Hour), Text: "蛋裂開了，人族「小橘」誕生了！"},
		{At: now.Add(-time.Hour).UTC(), Text: "小橘 開拓了一條新通道，獲得 60 XP。"},
	}
	golden(t, "log.txt", Log(lines))
	golden(t, "log_empty.txt", Log(nil))
}

func TestStatusline(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	pet := engine.Pet{Species: "human", Name: "GR騎士", Stage: 1, Level: 4, XP: 99, HP: 35, Satiety: 1000, SupplyDay: "2026-10-30", SupplyUsed: 3}
	ai := engine.AI{Turns: 40, Day: "2026-10-30", TodayTurns: 12}
	hungry := pet
	hungry.Satiety, hungry.SupplyDay = 120, "2026-10-29"
	var got []string
	for _, s := range []engine.State{
		{Pet: pet, AI: ai},
		{Pet: hungry, AI: engine.AI{Day: "2026-10-29", TodayTurns: 5}}, // 昨天的回合不算今天
		{Pet: pet, AI: ai, Dungeon: engine.Dungeon{Floor: 12, Deepest: 15}},
		{Pet: pet, AI: ai, Dungeon: engine.Dungeon{Floor: 12, Deepest: 15, Downed: true}},
		{},
	} {
		line := Statusline(s, c, now)
		if strings.Contains(line, "\n") {
			t.Fatalf("statusline 不可換行: %q", line)
		}
		got = append(got, line)
	}
	golden(t, "statusline.txt", strings.Join(got, "\n")+"\n")
}
