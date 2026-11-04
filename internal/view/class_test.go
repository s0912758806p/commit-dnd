package view

import (
	"testing"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

// status 的職業／進化列：未轉職、轉職、選了特化、最終階段，以及內容裡找不到的職業 id。
func TestStatusClassRows(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	pet := engine.Pet{Species: "human", Name: "GR騎士", Stage: 1, Level: 7, XP: 10, HP: 50, Satiety: 800, Mood: 600, Class: "knight"}
	ranked := pet
	ranked.Level, ranked.Stage, ranked.Spec, ranked.HP = 16, 2, "paladin", 80
	final := ranked
	final.Species, final.Level, final.Stage, final.Class, final.Spec, final.HP = "undead", 25, 3, "mage", "", 100
	unknown := pet
	unknown.Class = "bard"
	var got string
	for _, p := range []engine.Pet{pet, ranked, final, unknown} {
		got += Status(engine.State{Pet: p}, c, now) + "---\n"
	}
	golden(t, "status_class.txt", got)
}

func TestClass(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	var mix engine.Mix
	for kind, n := range map[string]int{"feat": 12, "fix": 3, "test": 9, "docs": 6, "chore": 2} {
		for range n {
			mix.Add(kind)
		}
	}
	egg := engine.Pet{Species: "elf", Name: "小精", Stage: 1, Level: 3, HP: 30, Mix: mix}
	classed := egg
	classed.Level, classed.Class = 8, "knight"
	ready := classed
	ready.Level, ready.Stage, ready.Scrolls = 15, 2, 1
	chosen := ready
	chosen.Spec, chosen.Level, chosen.Stage, chosen.Scrolls = "bulwark", 21, 3, 3
	// 迭代 6：這一輪取得終職、可以轉生；之前已轉生一次並取得過兩種終職。
	final := chosen
	final.Level, final.Final = 31, "citadel"
	final.Legacy.Rebirths, final.Legacy.FinalN = 1, 2
	final.Legacy.Finals[0] = engine.FinalEntry{ID: "citadel", Level: 2}
	final.Legacy.Finals[1] = engine.FinalEntry{ID: "inquisitor", Level: 1}
	cases := map[string]engine.Pet{
		"class_final.txt":    final,
		"class_unjudged.txt": egg,
		"class_empty.txt":    {Species: "elf", Name: "小精", Stage: 1, Level: 1},
		"class_judged.txt":   classed,
		"class_ready.txt":    ready,
		"class_chosen.txt":   chosen,
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			golden(t, name, Class(engine.State{Pet: p}, c))
		})
	}
	golden(t, "dungeon_egg.txt", Class(engine.State{}, c)) // 沒有寵物時同 dungeon 的提示
}

func TestStatusRebirthRow(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	p := engine.Pet{Species: "human", Name: "GR騎士", Stage: 1, Level: 3, XP: 10, HP: 30, Satiety: 800, Mood: 600}
	p.Legacy.Rebirths = 2
	golden(t, "status_rebirth.txt", Status(engine.State{Pet: p}, c, now))
}
