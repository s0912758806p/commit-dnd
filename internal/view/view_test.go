package view

import (
	"flag"
	"os"
	"path/filepath"
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
		Species: "slime", Name: "小橘", Level: 2, XP: 42, HP: 20, Satiety: 950, Mood: 500,
		SupplyDay: "2026-10-30", SupplyUsed: 1,
	}
	cases := map[string]engine.Pet{
		"status.txt":        pet,
		"status_hungry.txt": func() engine.Pet { p := pet; p.Species, p.Satiety, p.SupplyUsed = "owl", 120, 3; return p }(),
		"status_newday.txt": func() engine.Pet { p := pet; p.Species, p.SupplyDay, p.SupplyUsed = "drake", "2026-10-29", 3; return p }(),
		"status_knight.txt": func() engine.Pet { p := pet; p.Species, p.Name = "knight", "GR騎士"; return p }(),
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

func TestLog(t *testing.T) {
	lines := []engine.LogLine{
		{At: now.Add(-2 * time.Hour), Text: "蛋裂開了，史萊姆「小橘」誕生了！"},
		{At: now.Add(-time.Hour).UTC(), Text: "小橘 開拓了一條新通道，獲得 60 XP。"},
	}
	golden(t, "log.txt", Log(lines))
	golden(t, "log_empty.txt", Log(nil))
}
