package engine

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// growthPack 是 testPack 加上小型職業內容，數字方便手算：Lv 3 判定職業、Lv 5 可選特化、Lv 4／6 進化。
// testPack 本身沒有職業，既有測試的數值不受加成影響。
var growthPack = sync.OnceValue(func() *content.Pack {
	p := *testPack
	p.Growth = content.Growth{
		JudgeLevel: 3, SpecLevel: 5,
		Classes: []content.Class{
			{ID: "blade", Name: "劍士", Style: "開拓型", Kinds: []string{"feat"}, WeightPct: 100, Stats: content.Stats{Atk: 50},
				Specs: []content.Spec{{ID: "edge", Name: "刃", Stats: content.Stats{Atk: 50}}, {ID: "guard", Name: "盾", Stats: content.Stats{Def: 100}}}},
			{ID: "shield", Name: "守衛", Style: "守護型", Kinds: []string{"fix", "test"}, WeightPct: 200, Stats: content.Stats{Def: 100, HP: 50},
				Specs: []content.Spec{{ID: "wall", Name: "牆", Stats: content.Stats{HP: 100}}, {ID: "spike", Name: "刺", Stats: content.Stats{Atk: 100}}}},
			{ID: "scribe", Name: "書記", Style: "學者型", Kinds: []string{"other"}, WeightPct: 100,
				Specs: []content.Spec{{ID: "ink", Name: "墨", Stats: content.Stats{HP: 10}}, {ID: "quill", Name: "筆", Stats: content.Stats{Atk: 10}}}},
		},
		Stages:     []content.Stage{{Level: 1}, {Level: 4, Stats: content.Stats{Atk: 10, Def: 10, HP: 10}}, {Level: 6, Stats: content.Stats{Atk: 20, Def: 20, HP: 20}}},
		StageNames: map[string][]string{"slime": {"小史", "中史", "大史"}},
		Scroll:     content.Scroll{DropPct: 100, Max: 2},
		Logs: content.GrowthLogs{
			Judge: "轉職{class}（{style}）", SpecReady: "可選特化", SpecChosen: "特化{spec}", SpecDenied: "拒絕特化{spec}：{reason}",
			Evolve: "進化{stage}（{stage_n}）", ScrollDrop: "卷軸 {scrolls}/{scroll_max}", ScrollFull: "卷軸滿",
			Reroll: "重轉{class}", RerollSame: "仍是{class}", RerollDenied: "拒絕卷軸：{reason}",
		},
	}
	return &p
})

func grun(s State, events ...event.Event) Result {
	var r Result
	for _, e := range events {
		r = Apply(s, e, growthPack())
		s = r.State
	}
	return r
}

func classEv(action, spec string) event.Event {
	e := ev("class", t0)
	e.Args = map[string]string{"action": action, "spec": spec}
	return e
}

func mixOf(kinds ...string) Mix {
	var m Mix
	for _, k := range kinds {
		m.Add(k)
	}
	return m
}

func TestJudge(t *testing.T) {
	rep := func(k string, n int) []string { return slices.Repeat([]string{k}, n) }
	cases := []struct {
		name string
		mix  Mix
		want string
	}{
		{"feat 多 → 劍士", mixOf(rep("feat", 10)...), "blade"},
		{"權重：fix＋test 6 筆 ×200 勝過 feat 10 筆 ×100", mixOf(slices.Concat(rep("feat", 10), rep("fix", 3), rep("test", 3))...), "shield"},
		{"同分取較前面的", mixOf("feat", "feat", "chore", "ci"), "blade"},
		{"chore、ci 算 other", mixOf("chore", "ci", "build", "feat"), "scribe"},
		{"沒有紀錄時取第一個", Mix{}, "blade"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := Judge(tc.mix, growthPack()); !ok || got.ID != tc.want {
				t.Fatalf("got %q ok=%v", got.ID, ok)
			}
		})
	}
	if _, ok := Judge(mixOf("feat"), testPack); ok {
		t.Fatal("沒有職業內容時不應判定")
	}
}

func TestMixArg(t *testing.T) {
	m := mixOf("feat", "feat", "fix", "docs", "chore", "perf", "refactor", "test")
	if got := ParseMix(m.String()); got != m {
		t.Fatalf("往返: %v != %v（%s）", got, m, m.String())
	}
	if m.String() != "feat=2,fix=1,test=1,refactor=1,perf=1,docs=1,other=1" {
		t.Fatalf("%s", m.String())
	}
	// 不認得的類型算 other；負數、非數字、缺等號的略過
	if got, want := ParseMix("feat=3, chore=2,fix=-1,docs=x,test,style=1"), (Mix{3, 0, 0, 0, 0, 0, 3}); got != want {
		t.Fatalf("%v != %v", got, want)
	}
	if got := ParseMix("feat=99999999999999999999,other=2000000000"); got[0] != 0 || got[6] != mixCap {
		t.Fatalf("超出範圍要略過或夾住: %v", got)
	}
}

// 既有寵物（孵蛋時沒有 commit_mix）：重播已記錄的 commit 就會補算出組成；hatch 帶 mix 時從它開始累加。
func TestMixAccumulatesFromCommits(t *testing.T) {
	events := []event.Event{
		hatchEv(t0, "slime", "小橘"),
		commitEv(t0.Add(time.Minute), "feat", 1, "a"),
		commitEv(t0.Add(2*time.Minute), "feat", 1, "b"),
		commitEv(t0.Add(3*time.Minute), "chore", 1, "c"),
		commitEv(t0.Add(3*time.Minute), "chore", 1, "c"), // 同 hash 不重複計
		commitEv(t0.Add(4*time.Minute), "fix", 1, "d"),
	}
	if got := grun(State{}, events...).State.Pet.Mix; got != (Mix{2, 1, 0, 0, 0, 0, 1}) {
		t.Fatalf("補算: %v", got)
	}
	events[0].Args["mix"] = "test=5,docs=1"
	if got := grun(State{}, events...).State.Pet.Mix; got != (Mix{2, 1, 5, 0, 0, 1, 1}) {
		t.Fatalf("從 hatch 的 mix 累加: %v", got)
	}
	if got := grun(State{}, commitEv(t0, "feat", 1, "z")).State.Pet.Mix; got != (Mix{}) {
		t.Fatalf("孵蛋前的 commit 不計: %v", got)
	}
}

func TestClassJudgedAtJudgeLevel(t *testing.T) {
	s := grun(State{}, hatchEv(t0, "slime", "小橘")).State
	s.Pet.Mix = mixOf("fix", "fix", "fix", "feat", "feat", "feat")
	s.Pet.Level, s.Pet.XP = 2, 0
	r := grun(s, commitEv(t0, "other", 1, "a")) // 12 × 2 = 24 XP，還不到 Lv 3（44）
	if r.State.Pet.Class != "" || r.State.Pet.Level != 2 {
		t.Fatalf("Lv 2 不應轉職: %+v", r.State.Pet)
	}
	r = grun(r.State, commitEv(t0, "feat", 1000, "b")) // 累加後 fix 3 ×200 = 600 > feat 4 ×100
	p := r.State.Pet
	if p.Level < 3 || p.Class != "shield" || !strings.Contains(texts(r), "小橘 Lv3 | 轉職守衛（守護型）") || !slices.Contains(r.Fx, EffectClass) {
		t.Fatalf("%+v logs=%q fx=%v", p, texts(r), r.Fx)
	}
	if p.HP != MaxHP(p, growthPack()) || p.HP <= growthPack().MaxHP(p.Level) {
		t.Fatalf("升級後 HP 回滿到含加成的上限: %d", p.HP)
	}
	// 之後組成改變也不會自己換職業
	s = r.State
	s.Pet.Mix = mixOf(slices.Repeat([]string{"feat"}, 100)...)
	if p := grun(s, commitEv(t0, "feat", 1000, "c"), commitEv(t0, "feat", 1000, "d")).State.Pet; p.Class != "shield" || p.Level < 4 {
		t.Fatalf("職業只在判定等級決定一次: %+v", p)
	}
}

func TestEvolutionAndSpecReady(t *testing.T) {
	cases := []struct {
		from, stage int
		logs        string
	}{
		{3, 2, "小橘 Lv4 | 進化中史（2）"},
		{4, 2, "小橘 Lv5 | 可選特化"},
		{5, 3, "小橘 Lv6 | 進化大史（3）"},
	}
	for _, tc := range cases {
		s := hatched
		s.Pet.Class, s.Pet.Level, s.Pet.Stage, s.Pet.XP = "blade", tc.from, growthPack().StageFor(tc.from), growthPack().Cost(tc.from)-1
		s.Pet.SupplyDay, s.Pet.SupplyUsed = t0.Format(time.DateOnly), 3 // 不加倍，XP 只夠升一級
		r := grun(s, commitEv(t0, "other", 1, "x"))
		if r.State.Pet.Stage != tc.stage || !strings.HasSuffix(texts(r), tc.logs) {
			t.Fatalf("Lv %d→: stage=%d logs=%q", tc.from, r.State.Pet.Stage, texts(r))
		}
	}
	gp := growthPack()
	for level, want := range map[int]int{1: 1, 3: 1, 4: 2, 5: 2, 6: 3, 9: 3} {
		if got := gp.StageFor(level); got != want {
			t.Fatalf("StageFor(%d) = %d", level, got)
		}
	}
}

// 加成：職業＋特化＋進化相加，套用在不含裝備的基礎值上（小數捨去）。
func TestBonusApplies(t *testing.T) {
	gp := growthPack()
	s := hatched
	s.Pet.Level, s.Pet.Stage = 6, 3
	s.Dungeon.Weapon, s.Dungeon.Armor = Item{Name: "劍", Power: 3}, Item{Name: "甲", Power: 2}
	cases := []struct {
		class, spec  string
		atk, def, hp int
	}{
		// 基礎：攻擊 5+2×6 = 17、防禦 6、HP 20+5×5 = 45；第 3 階段 +20%
		{"", "", 17*120/100 + 3, 6*120/100 + 2, 45 * 120 / 100},
		{"blade", "", 17*170/100 + 3, 6*120/100 + 2, 45 * 120 / 100},
		{"blade", "edge", 17*220/100 + 3, 6*120/100 + 2, 45 * 120 / 100},
		{"blade", "wall", 17*170/100 + 3, 6*120/100 + 2, 45 * 120 / 100}, // 別的職業的特化不算
		{"shield", "wall", 17*120/100 + 3, 6*220/100 + 2, 45 * 270 / 100},
	}
	for _, tc := range cases {
		s.Pet.Class, s.Pet.Spec = tc.class, tc.spec
		if a, d, h := PetAtk(s, gp), PetDef(s, gp), MaxHP(s.Pet, gp); a != tc.atk || d != tc.def || h != tc.hp {
			t.Fatalf("%s/%s: atk=%d def=%d hp=%d，want %d %d %d", tc.class, tc.spec, a, d, h, tc.atk, tc.def, tc.hp)
		}
	}
	// 加成真的影響戰鬥：守衛（防禦 +100%）在 B1 被黏黏（攻擊 4）打，Lv 2 防禦 2×200% = 4 → 只扣 1
	s = atFloor(1, Monster{ID: "slimy", HP: 10, MaxHP: 10, Atk: 4})
	s.Pet.Level, s.Pet.Class, s.Pet.HP = 2, "shield", 30
	if hp := grun(s, aiTurn(t0)).State.Pet.HP; hp != 29 {
		t.Fatalf("HP = %d", hp)
	}
}

func TestChooseSpec(t *testing.T) {
	ready := hatched
	ready.Pet.Level, ready.Pet.Class, ready.Pet.HP = 5, "blade", 10
	cases := []struct {
		name string
		s    func() State
		spec string
		want string // 選完的 Spec
		logs string
	}{
		{"還沒轉職", func() State { s := ready; s.Pet.Class = ""; return s }, "edge", "", "拒絕特化edge：還沒轉職（Lv 3 依 commit 組成判定）"},
		{"等級不足", func() State { s := ready; s.Pet.Level = 4; return s }, "edge", "", "拒絕特化edge：Lv 5 才能選擇特化"},
		{"別的職業的特化", func() State { return ready }, "wall", "", "拒絕特化wall：劍士沒有這個特化"},
		{"不存在的特化", func() State { return ready }, "nope", "", "拒絕特化nope：劍士沒有這個特化"},
		{"選擇成功", func() State { return ready }, "edge", "edge", "特化刃"},
		{"已經選過", func() State { s := ready; s.Pet.Spec = "guard"; return s }, "edge", "guard", "拒絕特化edge：已經選了「盾」，要換請用轉職卷軸"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.s()
			r := grun(s, classEv("choose", tc.spec))
			if r.State.Pet.Spec != tc.want || texts(r) != tc.logs {
				t.Fatalf("spec=%q logs=%q", r.State.Pet.Spec, texts(r))
			}
			if tc.want == s.Pet.Spec && r.State.Pet != s.Pet {
				t.Fatalf("不合法時寵物不可改變: %+v", r.State.Pet)
			}
		})
	}
	if r := grun(State{}, classEv("choose", "edge")); texts(r) != "沒有寵物" {
		t.Fatalf("%q", texts(r))
	}
	if r := grun(ready, classEv("dance", "")); r.State.Pet != ready.Pet || len(r.Logs) != 0 {
		t.Fatal("不認得的 action 不做事")
	}
}

func TestScrollDropsFromBoss(t *testing.T) {
	boss := atFloor(3, Monster{ID: "king", HP: 1, MaxHP: 48, Atk: 14})
	r := grun(boss, toolUse(t0, "Edit"))
	if r.State.Pet.Scrolls != 1 || !strings.Contains(texts(r), "打倒 Boss 國王 解鎖 B3 | 卷軸 1/2") {
		t.Fatalf("scrolls=%d logs=%q", r.State.Pet.Scrolls, texts(r))
	}
	full := boss
	full.Pet.Scrolls = 2
	if r := grun(full, toolUse(t0, "Edit")); r.State.Pet.Scrolls != 2 || !strings.Contains(texts(r), "卷軸滿") {
		t.Fatalf("帶滿時不再增加: %d %q", r.State.Pet.Scrolls, texts(r))
	}
	if r := grun(atFloor(1, Monster{ID: "slimy", HP: 1, MaxHP: 10, Atk: 4}), toolUse(t0, "Edit")); r.State.Pet.Scrolls != 0 {
		t.Fatal("一般怪物不掉卷軸")
	}
	if r := run(boss, toolUse(t0, "Edit")); r.State.Pet.Scrolls != 0 {
		t.Fatal("drop_pct 為 0（testPack）時不掉")
	}
	// 機率掉落由種子決定：同一個狀態重播結果相同，不同 Seq 有掉有不掉
	half := *growthPack()
	half.Growth.Scroll.DropPct = 50
	got := map[int]bool{}
	for seq := range int64(40) {
		e := toolUse(t0, "Edit")
		e.Seq = seq + 1
		a, b := Apply(boss, e, &half).State.Pet.Scrolls, Apply(boss, e, &half).State.Pet.Scrolls
		if a != b {
			t.Fatal("同一事件結果不同")
		}
		got[a] = true
	}
	if !got[0] || !got[1] {
		t.Fatalf("50%% 應該有掉有不掉: %v", got)
	}
}

func TestReroll(t *testing.T) {
	base := hatched
	base.Pet.Level, base.Pet.XP, base.Pet.Class, base.Pet.HP, base.Pet.Scrolls = 5, 7, "blade", 1000, 1
	base.Pet.Mix = mixOf("feat", "feat")
	cases := []struct {
		name    string
		edit    func(p *Pet)
		class   string
		spec    string
		scrolls int
		logs    string
	}{
		{"還沒轉職", func(p *Pet) { p.Class = "" }, "", "", 1, "拒絕卷軸：還沒轉職（Lv 3 依 commit 組成判定）"},
		{"沒有卷軸", func(p *Pet) { p.Scrolls = 0 }, "blade", "", 0, "拒絕卷軸：沒有轉職卷軸（打倒 Boss 有機會掉落）"},
		{"結果相同且沒有特化：不消耗", func(p *Pet) {}, "blade", "", 1, "拒絕卷軸：依目前的 commit 組成還是劍士，卷軸沒有用掉"},
		{"結果相同但有特化：重置特化", func(p *Pet) { p.Spec = "edge" }, "blade", "", 0, "仍是劍士"},
		{"組成改變：換職業、清特化", func(p *Pet) { p.Spec = "edge"; p.Mix = mixOf("test", "test") }, "shield", "", 0, "重轉守衛"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.edit(&s.Pet)
			r := grun(s, classEv("reroll", ""))
			p := r.State.Pet
			if p.Class != tc.class || p.Spec != tc.spec || p.Scrolls != tc.scrolls || texts(r) != tc.logs {
				t.Fatalf("%+v logs=%q", p, texts(r))
			}
			if p.Level != 5 || p.XP != 7 || p.HP > MaxHP(p, growthPack()) {
				t.Fatalf("等級與 XP 保留、HP 夾在新上限內: %+v", p)
			}
		})
	}
}

// 重播一致：同一串事件（含孵蛋 mix、轉職、進化、卷軸、特化）跑兩次結果相同。
func TestGrowthReplayIsDeterministic(t *testing.T) {
	h := hatchEv(t0, "slime", "小橘")
	h.Args["mix"] = "feat=3,test=4"
	events := []event.Event{h}
	at := t0
	for i := range 30 {
		at = at.Add(17 * time.Minute)
		events = append(events, commitEv(at, "feat", i*7, string(rune('a'+i))), aiTurn(at), toolUse(at, "Edit"), toolUse(at, "Edit"), aiTurn(at))
	}
	events = append(events, classEv("choose", "wall"), classEv("reroll", ""))
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	a, b := grun(State{}, events...), grun(State{}, events...)
	if a.State != b.State || texts(a) != texts(b) {
		t.Fatalf("%+v\n!=\n%+v", a.State.Pet, b.State.Pet)
	}
	// Lv 3 時 test 4 ×200 勝過 feat → 守衛，選了特化「牆」；之後 feat 一路累加，用 Boss 掉的卷軸（2 張）重轉為劍士
	if p := a.State.Pet; p.Class != "blade" || p.Spec != "" || p.Scrolls != 1 || p.Stage != 3 || p.Mix.Total() != 37 || texts(a) != "重轉劍士" {
		t.Fatalf("%+v logs=%q", p, texts(a))
	}
}

// debug_xp 和 commit 走同一個升級流程（levelUp）：轉職、特化提示、進化都會觸發，HP 回滿到含加成的上限。
func TestDebugXPGrowsLikeCommit(t *testing.T) {
	gp := growthPack()
	xpTo := func(from, to int) int { // 從 Lv from（XP 0）升到 Lv to 需要的 XP
		n := 0
		for lv := from; lv < to; lv++ {
			n += gp.Cost(lv)
		}
		return n
	}
	cases := []struct {
		name      string
		from, to  int
		class     string
		stage     int
		wantLogs  []string
		forbidden string
	}{
		{"Lv 2 → 3：轉職", 2, 3, "shield", 1, []string{"小橘 Lv3", "轉職守衛（守護型）"}, "進化"},
		{"Lv 3 → 4：進化", 3, 4, "shield", 2, []string{"小橘 Lv4", "進化中史（2）"}, "轉職"},
		{"Lv 1 → 6：一次跨過所有門檻", 1, 6, "shield", 3, []string{"轉職守衛（守護型）", "進化中史（2）", "可選特化", "進化大史（3）"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := hatched
			s.Pet.Level, s.Pet.Stage, s.Pet.XP = tc.from, gp.StageFor(tc.from), 0
			s.Pet.Mix = mixOf("fix", "fix", "feat")
			if tc.from >= gp.Growth.JudgeLevel { // 已過判定等級的寵物早就有職業
				s.Pet.Class = "shield"
			}
			e := ev("debug_xp", t0)
			e.Args = map[string]string{"xp": strconv.Itoa(xpTo(tc.from, tc.to))}
			r := grun(s, e)
			p := r.State.Pet
			if p.Level != tc.to || p.Class != tc.class || p.Stage != tc.stage || p.HP != MaxHP(p, gp) {
				t.Fatalf("%+v", p)
			}
			for _, w := range tc.wantLogs {
				if !strings.Contains(texts(r), w) {
					t.Fatalf("缺少 %q: %q", w, texts(r))
				}
			}
			if tc.forbidden != "" && strings.Contains(texts(r), tc.forbidden) {
				t.Fatalf("不應出現 %q: %q", tc.forbidden, texts(r))
			}
		})
	}
}

// 轉職卷軸在 defeat() 裡掉落：合體技打倒 Boss 也會掉。
func TestComboBossKillDropsScroll(t *testing.T) {
	s := grun(atFloor(3, Monster{ID: "king", HP: 1, MaxHP: 48, Atk: 14}), toolUse(t0, "Read")).State // Read 不攻擊，但算 AI 活動
	r := grun(s, commitEv(t0, "feat", 0, "a"))
	if r.State.Party.Combos != 1 || r.State.Dungeon.Warp != 3 || r.State.Pet.Scrolls != 1 || !strings.Contains(texts(r), "打倒 Boss 國王 解鎖 B3 | 卷軸 1/2") {
		t.Fatalf("combos=%d scrolls=%d logs=%q", r.State.Party.Combos, r.State.Pet.Scrolls, texts(r))
	}
}
