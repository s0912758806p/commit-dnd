package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

func aiTurn(at time.Time) event.Event {
	e := ev("turn", at)
	e.Source = event.SourceClaude
	return e
}

func toolUse(at time.Time, tool string) event.Event {
	e := ev("tool_use", at)
	e.Source, e.Tool = event.SourceClaude, tool
	return e
}

func warpTo(at time.Time, floor string) event.Event {
	e := ev("warp", at)
	e.Args = map[string]string{"floor": floor}
	return e
}

// atFloor 回傳站在某層、這層還有怪物的狀態。
func atFloor(floor int, m Monster) State {
	s := hatched
	s.Dungeon.Floor, s.Dungeon.Deepest, s.Dungeon.Monster = floor, floor, m
	return s
}

func TestDescendOnFirstTurn(t *testing.T) {
	r := run(hatched, aiTurn(t0))
	d := r.State.Dungeon
	if d.Floor != 1 || d.Deepest != 1 || d.Monster != (Monster{ID: "slimy", HP: 10, MaxHP: 10, Atk: 4}) {
		t.Fatalf("%+v", d)
	}
	if texts(r) != "下探 B1 黏黏（水）" || r.State.Pet.HP != 20 {
		t.Fatalf("第一個回合只下探、不挨打: logs=%q hp=%d", texts(r), r.State.Pet.HP)
	}
}

func TestMonsterScalesWithFloor(t *testing.T) {
	s := hatched
	s.Dungeon.Floor, s.Dungeon.Cleared = 4, 2 // B4 打完了，下一層是 B5：成長 10%×4 → HP 14、攻擊 5
	d := run(s, aiTurn(t0)).State.Dungeon
	if d.Monster != (Monster{ID: "slimy", HP: 14, MaxHP: 14, Atk: 5}) {
		t.Fatalf("%+v", d.Monster)
	}
}

func TestMonsterAttacksOnLaterTurns(t *testing.T) {
	// Lv1：防禦 1，黏黏攻擊 4 → 每回合 -3
	p := run(hatched, aiTurn(t0), aiTurn(t0), aiTurn(t0)).State.Pet
	if p.HP != 14 {
		t.Fatalf("HP = %d", p.HP)
	}
}

func TestEditAttacksAndKills(t *testing.T) {
	// Lv1 攻擊 7：10 → 3 → 打倒，掉一件
	r := run(hatched, aiTurn(t0), toolUse(t0, "Edit"), toolUse(t0, "Read"), toolUse(t0, "Write"))
	d := r.State.Dungeon
	if d.Monster.ID != "" || d.Kills != 1 || d.BagN != 1 || d.Bag[0].Floor != 1 {
		t.Fatalf("%+v", d)
	}
	if texts(r) != "打倒 黏黏 | 掉落 1/10" {
		t.Fatalf("logs=%q", texts(r))
	}
	// 每層 2 隻：第二隻在同一層，打倒後才往下探
	r = run(r.State, aiTurn(t0))
	if d = r.State.Dungeon; d.Floor != 1 || d.Cleared != 1 || d.Monster.ID != "slimy" || texts(r) != "又遇到 B1 黏黏" {
		t.Fatalf("同一層的第二隻: %+v %q", d, texts(r))
	}
	s := r.State
	s.Dungeon.Monster.HP = 1
	if d = run(s, toolUse(t0, "Edit"), aiTurn(t0)).State.Dungeon; d.Floor != 2 || d.Cleared != 0 || d.Monster.ID != "slimy" {
		t.Fatalf("打完這層才往下探: %+v", d)
	}
}

func TestReadDoesNotAttack(t *testing.T) {
	s := atFloor(1, Monster{ID: "slimy", HP: 10, MaxHP: 10, Atk: 4})
	if d := run(s, toolUse(t0, "Read"), toolUse(t0, "Bash")).State.Dungeon; d.Monster.HP != 10 {
		t.Fatalf("%+v", d.Monster)
	}
}

func TestBossFloorAndWarpUnlock(t *testing.T) {
	s := hatched
	s.Dungeon.Floor, s.Dungeon.Cleared = 2, 2
	r := run(s, aiTurn(t0))
	// B3 是 Boss 層：20 × 120% × 200% = 48；攻擊 6 × 120% = 7 → × 200% = 14
	if m := r.State.Dungeon.Monster; m != (Monster{ID: "king", HP: 48, MaxHP: 48, Atk: 14}) || texts(r) != "Boss B3 國王" {
		t.Fatalf("%+v %q", m, texts(r))
	}
	s = r.State
	s.Dungeon.Monster.HP = 1
	r = run(s, toolUse(t0, "Edit"))
	if r.State.Dungeon.Warp != 3 || r.State.Dungeon.Cleared != 2 || !strings.HasPrefix(texts(r), "打倒 Boss 國王 解鎖 B3") {
		t.Fatalf("warp=%d logs=%q", r.State.Dungeon.Warp, texts(r))
	}
}

func TestDownedLosesBagAndStopsAdvancing(t *testing.T) {
	s := atFloor(3, Monster{ID: "king", HP: 48, MaxHP: 48, Atk: 14}) // Lv1 HP 20，每回合 -13
	s.Dungeon.Bag[0], s.Dungeon.Bag[1], s.Dungeon.BagN = Drop{Floor: 1, Seed: 1}, Drop{Floor: 2, Seed: 2}, 2
	r := run(s, aiTurn(t0), aiTurn(t0))
	d, p := r.State.Dungeon, r.State.Pet
	if !d.Downed || p.HP != 0 || d.Downs != 1 || d.BagN != 0 || d.Bag != ([BagSize]Drop{}) || texts(r) != "倒地 B3 遺失 2" {
		t.Fatalf("%+v hp=%d logs=%q", d, p.HP, texts(r))
	}
	after := run(r.State, aiTurn(t0), toolUse(t0, "Edit"), aiTurn(t0.Add(time.Hour)))
	if after.State.Dungeon.Floor != 3 || after.State.Dungeon.Monster != d.Monster || after.State.Pet.HP != 0 || len(after.Logs) != 0 {
		t.Fatalf("倒地中 AI 不能推進、攻擊或回血: %+v hp=%d", after.State.Dungeon, after.State.Pet.HP)
	}
}

func TestCommitRescuesOnSameFloor(t *testing.T) {
	s := atFloor(3, Monster{ID: "king", HP: 30, MaxHP: 48, Atk: 14})
	s.Pet.Level, s.Pet.HP, s.Dungeon.Downed = 5, 0, true // Lv5 HP 上限 40，這筆不會升級
	r := run(s, commitEv(t0, "other", 0, "a"))
	d, p := r.State.Dungeon, r.State.Pet
	// 救起時 40 × 50% = 20，再加 commit 回血 5
	if d.Downed || p.HP != 25 || d.Floor != 3 || d.Monster.HP != 30 || !strings.HasPrefix(texts(r), "救援 B3") {
		t.Fatalf("%+v hp=%d logs=%q", d, p.HP, texts(r))
	}
}

func TestCommitIdentifiesOldestDrop(t *testing.T) {
	s := hatched
	s.Pet.Level = 5
	s.Dungeon.Bag[0], s.Dungeon.Bag[1], s.Dungeon.BagN = Drop{Floor: 10, Seed: 42}, Drop{Floor: 1, Seed: 7}, 2
	r := run(s, commitEv(t0, "other", 0, "a"))
	d := r.State.Dungeon
	if d.BagN != 1 || d.Bag[0] != (Drop{Floor: 1, Seed: 7}) || d.Bag[1] != (Drop{}) {
		t.Fatalf("應鑑定最舊的一件並往前移: %+v", d.Bag[:2])
	}
	got := d.Weapon
	if got.Name == "" {
		got = d.Armor
	}
	// 強度 = 1 + 10/5 + 0～2
	if got.Name == "" || got.Power < 3 || got.Power > 5 || !strings.Contains(texts(r), "裝上 "+itemLabel(got)) {
		t.Fatalf("空的欄位一定換上: %+v logs=%q", d, texts(r))
	}

	s = r.State
	s.Dungeon.Weapon, s.Dungeon.Armor = Item{Kind: "weapon", Name: "神劍", Element: "fire", Power: 99}, Item{Kind: "armor", Name: "神甲", Element: "fire", Power: 99}
	r = run(s, commitEv(t0, "other", 0, "b"))
	if r.State.Dungeon.BagN != 0 || r.State.Dungeon.Weapon.Power != 99 || r.State.Dungeon.Armor.Power != 99 || !strings.Contains(texts(r), "賣掉 ") {
		t.Fatalf("比較弱的不換: %+v logs=%q", r.State.Dungeon, texts(r))
	}
}

func TestIdentifyIsDeterministic(t *testing.T) {
	for seed := range uint64(50) {
		a, b := identify(Drop{Floor: 7, Seed: seed}, testPack), identify(Drop{Floor: 7, Seed: seed}, testPack)
		if a != b || (a.Kind != "weapon" && a.Kind != "armor") || a.Power < 2 || a.Power > 4 {
			t.Fatalf("seed %d: %+v %+v", seed, a, b)
		}
	}
}

func TestElements(t *testing.T) {
	// 武器火剋草、被水剋；Lv1 攻擊 7 + 武器 3 = 10
	for elem, want := range map[string]int{"grass": 15, "water": 5, "fire": 10} {
		testPack.Monsters[0].Element = elem
		s := atFloor(1, Monster{ID: "slimy", HP: 100, MaxHP: 100, Atk: 4})
		s.Dungeon.Weapon = Item{Kind: "weapon", Name: "火劍", Element: "fire", Power: 3}
		if hp := run(s, toolUse(t0, "Edit")).State.Dungeon.Monster.HP; 100-hp != want {
			t.Errorf("武器火 vs 怪物%s：傷害 %d, want %d", elem, 100-hp, want)
		}
	}
	// 防具水被草剋、剋火；怪物攻擊 20 − 防禦（1 + 2）= 17
	for elem, want := range map[string]int{"grass": 25, "fire": 8, "water": 17} {
		testPack.Monsters[0].Element = elem
		s := atFloor(1, Monster{ID: "slimy", HP: 100, MaxHP: 100, Atk: 20})
		s.Pet.HP = 20
		s.Pet.Level = 1
		s.Dungeon.Armor = Item{Kind: "armor", Name: "水甲", Element: "water", Power: 2}
		if hp := run(s, aiTurn(t0)).State.Pet.HP; 20-hp != min(20, want) {
			t.Errorf("防具水 vs 怪物%s：傷害 %d, want %d", elem, 20-hp, want)
		}
	}
	testPack.Monsters[0].Element = "water"
}

func TestCamping(t *testing.T) {
	s := hatched
	s.Pet.Level, s.Pet.HP, s.Dungeon.LastActive = 5, 5, t0 // HP 上限 40
	// 7 小時 59 分：只有自然回血 15
	if hp := Apply(s, tick(t0.Add(8*time.Hour-time.Minute)), testPack).State.Pet.HP; hp != 20 {
		t.Fatalf("還沒滿 8 小時: HP %d", hp)
	}
	r := Apply(s, aiTurn(t0.Add(8*time.Hour)), testPack)
	if r.State.Pet.HP != 40 || !strings.HasPrefix(texts(r), "紮營") {
		t.Fatalf("閒置 8 小時應回滿: HP %d logs=%q", r.State.Pet.HP, texts(r))
	}
	s.Dungeon.Downed, s.Pet.HP = true, 0
	if hp := Apply(s, tick(t0.Add(24*time.Hour)), testPack).State.Pet.HP; hp != 0 {
		t.Fatalf("倒地中不會因紮營或自然回血復活: HP %d", hp)
	}
}

func TestActivityUpdatesLastActive(t *testing.T) {
	s := run(hatched, aiTurn(t0.Add(time.Hour)), tick(t0.Add(3*time.Hour))).State
	if !s.Dungeon.LastActive.Equal(t0.Add(time.Hour)) {
		t.Fatalf("tick 不算活動: %v", s.Dungeon.LastActive)
	}
}

func TestWarp(t *testing.T) {
	s := atFloor(7, Monster{ID: "slimy", HP: 5, MaxHP: 10, Atk: 4})
	s.Dungeon.Warp = 6
	cases := []struct {
		floor string
		want  int
		log   string
	}{
		{"3", 3, "傳送 B3"},
		{"6", 6, "傳送 B6"},
		{"0", 0, "回入口"},
		{"9", 7, "拒絕 B9：還沒打倒 B9 的 Boss"},
		{"4", 7, "拒絕 B4：B4 不是傳送點"},
		{"x", 7, "拒絕 Bx：樓層必須是數字"},
	}
	for _, tc := range cases {
		r := run(s, warpTo(t0, tc.floor))
		d := r.State.Dungeon
		if d.Floor != tc.want || texts(r) != tc.log || (d.Floor != 7 && d.Monster != (Monster{})) {
			t.Errorf("warp %s: floor=%d monster=%+v logs=%q", tc.floor, d.Floor, d.Monster, texts(r))
		}
	}
	s.Dungeon.Downed = true
	if r := run(s, warpTo(t0, "3")); r.State.Dungeon.Floor != 7 || texts(r) != "拒絕 B3：倒地中不能傳送" {
		t.Errorf("倒地中: %q", texts(r))
	}
}

// 迭代 3 的驗收：只用 AI 工作、不 commit，寵物遲早倒地；commit 後從同一層繼續。
func TestAIOnlyEventuallyDownsThenCommitContinues(t *testing.T) {
	s := hatched
	at := t0
	for i := 0; i < 500 && !s.Dungeon.Downed; i++ {
		at = at.Add(3 * time.Minute)
		s = run(s, aiTurn(at), toolUse(at, "Edit"), toolUse(at, "Read"), toolUse(at, "Write")).State
	}
	if !s.Dungeon.Downed {
		t.Fatalf("500 個 AI 回合都沒倒地: %+v pet=%+v", s.Dungeon, s.Pet)
	}
	floor := s.Dungeon.Floor
	s = run(s, commitEv(at.Add(time.Minute), "fix", 3, "rescue")).State
	if s.Dungeon.Downed || s.Dungeon.Floor != floor || s.Pet.HP <= 0 {
		t.Fatalf("commit 後應在 B%d 繼續: %+v hp=%d", floor, s.Dungeon, s.Pet.HP)
	}
}
