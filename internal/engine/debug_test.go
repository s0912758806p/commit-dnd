package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

func debugEv(typ, key, val string, at time.Time) event.Event {
	e := ev(typ, at)
	e.Args = map[string]string{key: val}
	return e
}

func TestDebugEvents(t *testing.T) {
	// B2 有一隻怪物、這層打倒 1 隻、最深 B5、背包 1 件
	deep := hatched
	deep.Dungeon = Dungeon{Floor: 2, Deepest: 5, Cleared: 1, Monster: Monster{ID: "slimy", HP: 3, MaxHP: 10, Atk: 4}, Bag: [BagSize]Drop{{Floor: 2, Seed: 7}}, BagN: 1}
	downed := deep
	downed.Dungeon.Downed = true

	cases := []struct {
		name  string
		s     State
		e     event.Event
		check func(t *testing.T, before State, r Result)
	}{
		{"xp 照正常流程連續升級", hatched, debugEv("debug_xp", "xp", "70", t0), func(t *testing.T, _ State, r Result) {
			// 18 + 44 = 62 → Lv 3，剩 8；HP 回滿 Lv 3 的上限
			p := r.State.Pet
			if p.Level != 3 || p.XP != 8 || p.HP != testPack.MaxHP(3) {
				t.Fatalf("Lv %d XP %d HP %d", p.Level, p.XP, p.HP)
			}
			if got := texts(r); !strings.Contains(got, "獲得 70 XP") || strings.Count(got, "小橘 Lv") != 2 {
				t.Fatalf("logs %q", got)
			}
		}},
		{"xp 不到下一級只累積", hatched, debugEv("debug_xp", "xp", "5", t0), func(t *testing.T, _ State, r Result) {
			if p := r.State.Pet; p.Level != 1 || p.XP != 5 || p.Commits != 0 || p.SupplyUsed != 0 {
				t.Fatalf("%+v", p)
			}
		}},
		{"xp 不合法不變", hatched, debugEv("debug_xp", "xp", "-3", t0), unchanged},
		{"xp 不是數字不變", hatched, debugEv("debug_xp", "xp", "很多", t0), unchanged},
		{"floor 跳到更深的樓層", deep, debugEv("debug_floor", "floor", "40", t0), func(t *testing.T, before State, r Result) {
			d := r.State.Dungeon
			if d.Floor != 40 || d.Deepest != 40 || d.Cleared != 0 || d.Monster != (Monster{}) {
				t.Fatalf("%+v", d)
			}
			if d.BagN != 1 || d.Warp != before.Dungeon.Warp {
				t.Fatalf("背包與傳送點不應變: %+v", d)
			}
			if got := texts(r); !strings.Contains(got, "被傳到B40") {
				t.Fatalf("logs %q", got)
			}
		}},
		{"floor 往回跳不降低最深樓層", deep, debugEv("debug_floor", "floor", "1", t0), func(t *testing.T, _ State, r Result) {
			if d := r.State.Dungeon; d.Floor != 1 || d.Deepest != 5 || d.Cleared != 0 {
				t.Fatalf("%+v", d)
			}
		}},
		{"floor 0 回入口", deep, debugEv("debug_floor", "floor", "0", t0), func(t *testing.T, _ State, r Result) {
			if d := r.State.Dungeon; d.Floor != 0 || d.Monster.ID != "" || !strings.Contains(texts(r), "被傳到入口") {
				t.Fatalf("%+v %q", d, texts(r))
			}
		}},
		{"floor 不改變倒地狀態", downed, debugEv("debug_floor", "floor", "9", t0), func(t *testing.T, _ State, r Result) {
			if d := r.State.Dungeon; d.Floor != 9 || !d.Downed {
				t.Fatalf("%+v", d)
			}
		}},
		{"floor 不合法不變", deep, debugEv("debug_floor", "floor", "-1", t0), unchanged},
		{"沒有寵物時不變", State{}, debugEv("debug_xp", "xp", "100", t0), func(t *testing.T, _ State, r Result) {
			if r.State.Pet.Hatched() || texts(r) != "沒有寵物" {
				t.Fatalf("%+v %q", r.State.Pet, texts(r))
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, tc.s, Apply(tc.s, tc.e, testPack))
		})
	}
}

// unchanged 檢查除了 Apply 固定會更新的欄位以外狀態不變，並留下一句日誌。
func unchanged(t *testing.T, before State, r Result) {
	t.Helper()
	after := r.State
	after.V, after.ContentHash, after.LastSeq, after.UpdatedAt = before.V, before.ContentHash, before.LastSeq, before.UpdatedAt
	if after != before {
		t.Fatalf("狀態不應改變:\n%+v\n%+v", before, after)
	}
	if len(r.Logs) != 1 {
		t.Fatalf("應留一句日誌: %q", texts(r))
	}
}

// 跳樓層後的第一個 AI 回合在新樓層遇到怪物，強度照該層成長。
func TestDebugFloorThenTurn(t *testing.T) {
	r := run(hatched, debugEv("debug_floor", "floor", "2", t0), aiTurn(t0.Add(time.Minute)))
	d := r.State.Dungeon
	if d.Floor != 2 || d.Monster.ID != "slimy" || d.Monster.MaxHP != 11 { // 10 × 110%
		t.Fatalf("%+v", d)
	}
}
