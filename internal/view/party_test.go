package view

import (
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
)

// partyState 是一個收服過幾隻的存檔：隊伍 2 隻，候補 1 隻，遇過一隻 Boss 與一隻沒收服的怪物。
func partyState() engine.State {
	s := engine.State{Pet: engine.Pet{Species: "human", Name: "小橘", Stage: 1, Level: 12, HP: 70, Satiety: 800, Mood: 600}}
	for i, en := range []engine.DexEntry{
		{ID: "null_slime", Seen: 9, Caught: 2, BestFloor: 5},
		{ID: "race_bat", Seen: 7, Caught: 1, BestFloor: 8},
		{ID: "off_by_one", Seen: 12},
		{ID: "bug_king", Seen: 1},
		{ID: "deadlock", Seen: 4, Caught: 1, BestFloor: 21},
	} {
		s.Dex.Entries[i] = en
		s.Dex.N = i + 1
	}
	s.Party.Members, s.Party.N = [engine.PartySize]string{"race_bat", "deadlock"}, 2
	return s
}

func TestDex(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "dex.txt", Dex(partyState(), c))
	s := partyState()
	s.Dex, s.Party = engine.Dex{}, engine.Party{}
	golden(t, "dex_empty.txt", Dex(s, c))
	golden(t, "dex_egg.txt", Dex(engine.State{}, c))
}

func TestParty(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	ready := partyState()
	ready.Party.LastAI = now.Add(-10 * time.Minute)
	stale := partyState()
	stale.Party.LastAI = now.Add(-31 * time.Minute)
	empty := partyState()
	empty.Dex, empty.Party = engine.Dex{}, engine.Party{}
	got := Party(ready, c, now) + "---\n" + Party(stale, c, now) + "---\n" + Party(empty, c, now) + "---\n" + Party(engine.State{}, c, now)
	golden(t, "party.txt", got)
}

func TestDungeonParty(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	s := partyState()
	s.Dungeon = engine.Dungeon{Floor: 12, Deepest: 12, Warp: 10, Cleared: 1, Monster: engine.Monster{ID: "deadlock", HP: 20, MaxHP: 33, Atk: 10}}
	golden(t, "dungeon_party.txt", Dungeon(s, c))
}
