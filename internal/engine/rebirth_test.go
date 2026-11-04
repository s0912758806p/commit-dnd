package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// rebirthPack 是 growthPack 加上終職（每個特化一種、能力各不相同）與轉生：Lv 7 取得終職、Lv 7 可轉生。
var rebirthPack = sync.OnceValue(func() *content.Pack {
	p := *growthPack()
	g := p.Growth
	g.FinalLevel = 7
	g.Finals = []content.Final{
		{ID: "edge_king", Spec: "edge", Name: "刃王", Ability: "combo_pct", PerLevel: 50},
		{ID: "fortress", Spec: "guard", Name: "要塞", Ability: "keep_on_down", PerLevel: 2},
		{ID: "saint", Spec: "wall", Name: "聖者", Ability: "rescue_hp_pct", PerLevel: 20},
		{ID: "hunter", Spec: "spike", Name: "獵人", Ability: "capture_pct", PerLevel: 10},
		{ID: "scholar", Spec: "ink", Name: "學者", Ability: "kind_xp_pct", PerLevel: 50, Kind: "other"},
		{ID: "smith", Spec: "quill", Name: "工匠", Ability: "item_power", PerLevel: 1},
	}
	g.Rebirth = content.Rebirth{Level: 7, StatsPct: 10, XPPct: 20}
	g.Logs.Final = "終職{final} Lv{final_level}：{ability}"
	g.Logs.FinalUp = "強化{final} Lv{final_level}：{ability}"
	g.Logs.Rebirth = "轉生{rebirths} +{stats}% +{xp}%"
	g.Logs.RebirthDenied = "拒絕轉生：{reason}"
	p.Growth = g
	return &p
})

func rrun(s State, events ...event.Event) Result {
	var r Result
	for _, e := range events {
		r = Apply(s, e, rebirthPack())
		s = r.State
	}
	return r
}

func rebirthEv(at time.Time) event.Event { return ev("rebirth", at) }

// lv6 是 Lv 6 的劍士；cost(6) = 185，給 185 XP 就升到 Lv 7。
func lv6(spec string) State {
	s := hatched
	s.Pet.Level, s.Pet.Class, s.Pet.Spec, s.Pet.Stage = 6, "blade", spec, 3
	s.Pet.HP = MaxHP(s.Pet, rebirthPack())
	return s
}

func TestFinalAtFinalLevel(t *testing.T) {
	r := rrun(lv6("edge"), debugEv("debug_xp", "xp", "185", t0))
	p := r.State.Pet
	if p.Level != 7 || p.Final != "edge_king" || p.Legacy.FinalN != 1 || p.Legacy.FinalLevel("edge_king") != 1 {
		t.Fatalf("%+v", p)
	}
	if !strings.Contains(texts(r), "終職刃王 Lv1：合體技倍率 +50%") {
		t.Fatalf("logs=%q", texts(r))
	}
	if again := rrun(r.State, debugEv("debug_xp", "xp", "300", t0)); again.State.Pet.Legacy.FinalLevel("edge_king") != 1 {
		t.Fatal("同一輪只取得一次終職")
	}
}

func TestFinalWaitsForSpec(t *testing.T) {
	s := rrun(lv6(""), debugEv("debug_xp", "xp", "185", t0)).State
	if s.Pet.Level != 7 || s.Pet.Final != "" {
		t.Fatalf("沒有特化不能取得終職: %+v", s.Pet)
	}
	r := rrun(s, classEv("choose", "guard"))
	if r.State.Pet.Final != "fortress" || !strings.Contains(texts(r), "終職要塞 Lv1") {
		t.Fatalf("選特化的那一刻取得終職: %+v %q", r.State.Pet, texts(r))
	}
}

func TestRebirthDenied(t *testing.T) {
	for name, tc := range map[string]struct {
		s      State
		reason string
	}{
		"沒有終職": {lv6("edge"), "要先在 Lv 7 取得終職"},
		"倒地中": {func() State {
			s := rrun(lv6("edge"), debugEv("debug_xp", "xp", "185", t0)).State
			s.Dungeon.Downed = true
			return s
		}(), "倒地中不能轉生"},
	} {
		r := rrun(tc.s, rebirthEv(t0))
		if r.State.Pet != tc.s.Pet || !strings.Contains(texts(r), tc.reason) {
			t.Errorf("%s: pet 不應改變，logs=%q", name, texts(r))
		}
	}
}

func TestRebirthResetsAndKeeps(t *testing.T) {
	s := rrun(lv6("edge"), debugEv("debug_xp", "xp", "185", t0)).State
	s.Pet.Scrolls, s.Pet.Mix = 2, mixOf("feat", "feat", "fix")
	s.Dungeon = Dungeon{Floor: 33, Deepest: 40, Warp: 30, Kills: 99, Downs: 4, Cleared: 3,
		Weapon: Item{Kind: "weapon", Name: "劍", Power: 9}, BagN: 1, LastActive: t0}
	s.Dungeon.Bag[0] = Drop{Floor: 3, Seed: 1}
	s.Party.N, s.Party.Members[0] = 1, "slimy"
	party, dex := s.Party, s.Dex

	r := rrun(s, rebirthEv(t0))
	p, d := r.State.Pet, r.State.Dungeon
	if p.Level != 1 || p.XP != 0 || p.Class != "" || p.Spec != "" || p.Final != "" || p.Stage != 1 || p.Scrolls != 0 {
		t.Fatalf("等級、職業、特化、終職、進化、卷軸要重置: %+v", p)
	}
	if p.Name != s.Pet.Name || p.Species != s.Pet.Species || p.Mix != s.Pet.Mix || p.Legacy.Rebirths != 1 || p.Legacy.FinalLevel("edge_king") != 1 {
		t.Fatalf("種族、名字、組成、終職紀錄要保留: %+v", p)
	}
	if d != (Dungeon{Kills: 99, Downs: 4, LastActive: t0}) {
		t.Fatalf("地城只保留戰績與活動時間: %+v", d)
	}
	if r.State.Party != party || r.State.Dex != dex {
		t.Fatal("圖鑑與隊伍要保留")
	}
	// Lv 1 HP 基礎 20，轉生一次 +10% → 22
	if p.HP != 22 || MaxHP(p, rebirthPack()) != 22 || texts(r) != "轉生1 +10% +20%" {
		t.Fatalf("hp=%d logs=%q", p.HP, texts(r))
	}
}

func TestRebirthBonuses(t *testing.T) {
	base := hatched
	base.Pet.Level = 5 // 避免升級
	reborn := base
	reborn.Pet.Legacy.Rebirths = 2
	if a, b := PetAtk(base, rebirthPack()), PetAtk(reborn, rebirthPack()); b != pct(a, 120) {
		t.Fatalf("轉生兩次攻擊 +20%%: %d → %d", a, b)
	}
	// other 10 XP：轉生兩次 +40% → 14，補給 ×2 → 28
	gain := func(s State) int { return rrun(s, commitEv(t0, "other", 0, "x")).State.Pet.XP - s.Pet.XP }
	if g0, g2 := gain(base), gain(reborn); g0 != 20 || g2 != 28 {
		t.Fatalf("XP：%d、%d", g0, g2)
	}
}

func TestSameFinalLevelsUpAcrossRebirths(t *testing.T) {
	s := rrun(lv6("edge"), debugEv("debug_xp", "xp", "185", t0), rebirthEv(t0)).State
	// 第二輪：重新升到 Lv 7（職業在 Lv 3 依組成判定；這隻沒有組成 → 書記），選 ink 取得學者
	s = rrun(s, debugEv("debug_xp", "xp", "200", t0)).State
	s.Pet.Class = "blade" // 讓第二輪也走劍士，才能再次取得刃王
	r := rrun(s, debugEv("debug_xp", "xp", "1000", t0), classEv("choose", "edge"))
	p := r.State.Pet
	if p.Legacy.FinalLevel("edge_king") != 2 || p.Legacy.FinalN != 1 || !strings.Contains(texts(r), "強化刃王 Lv2：合體技倍率 +100%") {
		t.Fatalf("%+v logs=%q", p.Legacy, texts(r))
	}
	if Ability(p, rebirthPack(), "combo_pct") != 100 {
		t.Fatal("能力 = per_level × 等級")
	}
}

func legacyWith(c *content.Pack, level int, ids ...string) Legacy {
	var l Legacy
	for _, id := range ids {
		l.Finals[l.FinalN] = FinalEntry{ID: id, Level: level}
		l.FinalN++
	}
	return l
}

func TestAbilitiesInRules(t *testing.T) {
	c := rebirthPack()
	with := func(id string) State {
		s := hatched
		s.Pet.Level = 5
		s.Pet.Legacy = legacyWith(c, 1, id)
		return s
	}

	// 要塞：倒地時保留最舊的 2 件
	s := with("fortress")
	s.Dungeon = Dungeon{Floor: 3, Deepest: 3, Monster: Monster{ID: "king", HP: 99, MaxHP: 99, Atk: 999}}
	s.Dungeon.Bag[0], s.Dungeon.Bag[1], s.Dungeon.Bag[2], s.Dungeon.BagN = Drop{Seed: 1}, Drop{Seed: 2}, Drop{Seed: 3}, 3
	d := rrun(s, aiTurn(t0)).State.Dungeon
	if !d.Downed || d.BagN != 2 || d.Bag[0].Seed != 1 || d.Bag[1].Seed != 2 || d.Bag[2] != (Drop{}) {
		t.Errorf("keep_on_down: %+v", d)
	}

	// 聖者：救起時 HP 50% + 20%（Lv5 上限 40 → 28，再加 commit 回血 5）
	s = with("saint")
	s.Pet.HP, s.Dungeon.Downed = 0, true
	if hp := rrun(s, commitEv(t0, "other", 0, "a")).State.Pet.HP; hp != 33 {
		t.Errorf("rescue_hp_pct: HP %d", hp)
	}

	// 獵人：收服機率 +10
	s = with("hunter")
	s.Dungeon.Monster = Monster{ID: "slimy", HP: 10, MaxHP: 10}
	if got := captureChance(s, c); got != 50 {
		t.Errorf("capture_pct: %d", got)
	}

	// 學者：other 類 commit XP +50%（10 → 15，補給 ×2 → 30）
	s = with("scholar")
	if g := rrun(s, commitEv(t0, "other", 0, "x")).State.Pet.XP; g != 30 {
		t.Errorf("kind_xp_pct: %d", g)
	}

	// 工匠：鑑定出的裝備強度 +1
	plain, smith := hatched, with("smith")
	plain.Pet.Level = 5
	for _, st := range []*State{&plain, &smith} {
		st.Dungeon.Bag[0], st.Dungeon.BagN = Drop{Floor: 10, Seed: 42}, 1
	}
	a := rrun(plain, commitEv(t0, "other", 0, "a")).State.Dungeon
	b := rrun(smith, commitEv(t0, "other", 0, "a")).State.Dungeon
	if a.Weapon.Power+a.Armor.Power+1 != b.Weapon.Power+b.Armor.Power {
		t.Errorf("item_power: %+v vs %+v", a, b)
	}

	// 刃王：合體技倍率 +50
	if got, want := ComboPct(with("edge_king"), c), ComboPct(hatched, c)+50; got != want {
		t.Errorf("combo_pct: %d want %d", got, want)
	}
}

// 用正式內容檢查其餘能力：每種能力都要在規則裡生效。
func TestAbilitiesWithRealContent(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Growth.Finals) != 12 {
		t.Fatalf("12 種終職，實際 %d", len(c.Growth.Finals))
	}
	pet := Pet{Species: "human", Name: "測", Level: 30, Stage: 3, Satiety: 800, HP: 100}
	with := func(id string) Pet { p := pet; p.Legacy = legacyWith(c, 1, id); return p }

	if got := SupplyPerDay(with("archsage"), c); got != c.Balance.Supply.CommitsPerDay+1 {
		t.Errorf("supply_commits: %d", got)
	}
	// 星界旅人：飽足下降 −25%（每小時 10 → 7）
	at := t0.Add(10 * time.Hour)
	drop := func(p Pet) int {
		s := State{V: StateVersion, UpdatedAt: t0, Pet: p}
		return p.Satiety - Apply(s, tick(at), c).State.Pet.Satiety
	}
	if a, b := drop(pet), drop(with("starwalker")); a != 100 || b != 70 {
		t.Errorf("satiety_slow: %d → %d", a, b)
	}
	// 萬靈藥師：commit 多回 10 HP
	heal := func(p Pet) int {
		p.HP = 10
		s := State{V: StateVersion, UpdatedAt: t0, Pet: p}
		return Apply(s, commitEv(t0, "other", 0, "h"), c).State.Pet.HP - 10
	}
	if a, b := heal(pet), heal(with("panacea")); b != a+10 {
		t.Errorf("commit_heal: %d → %d", a, b)
	}
	// 元素主宰：相剋時傷害倍率 +25
	fire, _ := c.MonsterByID("race_bat")
	grass, _ := c.MonsterByID("off_by_one")
	s := State{Pet: with("elemental_lord"), Dungeon: Dungeon{Weapon: Item{Kind: "weapon", Element: "fire", Power: 1}}}
	if got := petElementPct(s, grass, c); got != c.Balance.Dungeon.ElementAdvantagePct+25 {
		t.Errorf("element_pct 相剋: %d", got)
	}
	if got := petElementPct(s, fire, c); got != 100 {
		t.Errorf("element_pct 不相剋時不加: %d", got)
	}
	// 傳奇傭兵：隊伍攻擊 +25%
	st := State{Pet: pet}
	st.Party.N, st.Party.Members[0] = 1, "null_slime"
	st.Dex.Entries[0], st.Dex.N = DexEntry{ID: "null_slime", Caught: 1, BestFloor: 40}, 1
	warlord := st
	warlord.Pet = with("warlord")
	if a, b := PartyAtk(st, c), PartyAtk(warlord, c); b != pct(a, 125) || a == 0 {
		t.Errorf("party_pct: %d → %d", a, b)
	}
	// 寶藏獵人：轉職卷軸掉落機率 +25（數值本身由 Ability 提供，dropScroll 使用）
	if Ability(with("treasure_hunter"), c, "scroll_pct") != 25 {
		t.Error("scroll_pct")
	}
	for _, f := range c.Growth.Finals {
		if f.AbilityText(2) == "" || strings.Contains(f.AbilityText(2), "%!") {
			t.Errorf("%s 的能力說明格式錯誤: %q", f.ID, f.AbilityText(2))
		}
	}
}

// DEVPLAN 迭代 6：檢查所有終職都不會讓 AI 單獨回血或免除倒地。
func TestAbilitiesNeverLetAIHeal(t *testing.T) {
	c, _ := content.Default()
	var all []string
	for _, f := range c.Growth.Finals {
		all = append(all, f.ID)
	}
	pet := Pet{Species: "human", Name: "測", Class: "ranger", Spec: "marksman", Final: "dawn_archer",
		Level: 30, Stage: 3, Satiety: 800, Legacy: legacyWith(c, 5, all...)}
	m, _ := c.MonsterByID("deadlock")
	at := t0
	// 時間經過的自然回血與紮營不算 AI 回血：AI 事件後的 HP 不可超過同一時間只推進時間（tick）的 HP。
	ai := func(s State) State {
		for range 50 {
			at = at.Add(time.Minute)
			for _, e := range []event.Event{toolUse(at, "Edit"), toolUse(at, "Write"), aiTurn(at)} {
				timeOnly := Apply(s, tick(e.At), c).State
				s = Apply(s, e, c).State
				if s.Pet.HP > timeOnly.Pet.HP || (timeOnly.Dungeon.Downed && !s.Dungeon.Downed) {
					t.Fatalf("%s 之後 HP %d、倒地 %v；只推進時間是 HP %d、倒地 %v", e.Type, s.Pet.HP, s.Dungeon.Downed, timeOnly.Pet.HP, timeOnly.Dungeon.Downed)
				}
			}
		}
		return s
	}
	downed := State{V: StateVersion, UpdatedAt: t0, Pet: pet,
		Dungeon: Dungeon{Floor: 30, Downed: true, Monster: Monster{ID: m.ID, HP: 50, MaxHP: 50, Atk: 999}, LastActive: t0}}
	downed.Pet.HP = 0
	if s := ai(downed); !s.Dungeon.Downed || s.Pet.HP != 0 {
		t.Fatalf("倒地中 AI 不能救起或回血: %+v hp=%d", s.Dungeon, s.Pet.HP)
	}
	fighting := downed
	fighting.Dungeon.Downed, fighting.Pet.HP = false, 50
	ai(fighting)
}

// 迭代 6 的驗收：用除錯 XP 快轉到 Lv 30，完成兩次轉生，能力疊加正確，且推進深度明顯增加。
func TestTwoRebirthsStackAndGoDeeper(t *testing.T) {
	c, _ := content.Default()
	at := t0
	var s State
	apply := func(e event.Event) Result {
		r := Apply(s, e, c)
		s = r.State
		return r
	}
	hatch := ev("hatch", at)
	hatch.Args = map[string]string{"species": "human", "name": "測", "mix": "feat=30,fix=5,test=2,docs=8"}
	apply(hatch)
	for life := 1; life <= 3; life++ {
		apply(debugEv("debug_xp", "xp", "5000", at)) // 到 Lv 15 以上
		if s.Pet.Class != "ranger" {
			t.Fatalf("第 %d 輪職業 %q", life, s.Pet.Class)
		}
		apply(classEv("choose", "marksman"))
		apply(debugEv("debug_xp", "xp", "50000", at)) // 到 Lv 30 以上
		if s.Pet.Final != "dawn_archer" || s.Pet.Legacy.FinalLevel("dawn_archer") != life {
			t.Fatalf("第 %d 輪終職: %q Lv %d", life, s.Pet.Final, s.Pet.Legacy.FinalLevel("dawn_archer"))
		}
		if life < 3 {
			if r := apply(rebirthEv(at)); s.Pet.Level != 1 || s.Pet.Legacy.Rebirths != life || !strings.Contains(texts(r), "轉生") {
				t.Fatalf("第 %d 次轉生: %+v %q", life, s.Pet, texts(r))
			}
		}
	}
	if s.Pet.Legacy.Rebirths != 2 || Ability(s.Pet, c, "combo_pct") != 150 || RebirthStats(s.Pet, c).Atk != 20 || RebirthXPPct(s.Pet, c) != 40 {
		t.Fatalf("疊加: %+v", s.Pet.Legacy)
	}

	// 同樣從 Lv 1 開始、同樣玩 20 天（每天 8 筆 feat commit、40 個 AI 回合、每回合 2 次 Edit），
	// 轉生兩次的寵物升級更快（XP +40%）、基礎能力 +20%、合體技倍率更高，應明顯走得更深。
	play := func(l Legacy, party Party, dex Dex) (deepest, level int) {
		st := State{}
		seq := int64(0)
		step := func(e event.Event) {
			seq++
			e.Seq = seq
			st = Apply(st, e, c).State
		}
		h := ev("hatch", t0)
		h.Args = map[string]string{"species": "human", "name": "測", "mix": "feat=30,fix=5,docs=8"}
		step(h)
		st.Pet.Legacy, st.Party, st.Dex = l, party, dex
		st.Pet.HP = MaxHP(st.Pet, c)
		for day := range 20 {
			base := t0.Add(time.Duration(day) * 24 * time.Hour)
			for i := range 48 {
				at := base.Add(time.Duration(i) * 12 * time.Minute)
				if i%6 == 5 {
					e := commitEv(at, "feat", 60, "")
					e.Commit.Hash = strings.Repeat("x", day+1) + string(rune('a'+i))
					step(e)
					continue
				}
				step(toolUse(at, "Edit"))
				step(toolUse(at, "Edit"))
				step(aiTurn(at))
			}
		}
		return st.Dungeon.Deepest, st.Pet.Level
	}
	// 轉生保留圖鑑與隊伍（使用者的決定）：前幾輪在 B90 附近收服的三隻隊員會跟著轉生。
	var party Party
	var dex Dex
	for i, id := range []string{"flaky", "monolith", "hotfix"} {
		party.Members[i] = id
		dex.Entries[i] = DexEntry{ID: id, Seen: 5, Caught: 1, BestFloor: 90}
	}
	party.N, dex.N = 3, 3

	f, fl := play(Legacy{}, Party{}, Dex{})
	r, rl := play(s.Pet.Legacy, Party{}, Dex{})
	rp, rpl := play(s.Pet.Legacy, party, dex)
	t.Logf("玩 20 天：沒轉生 Lv %d／B%d；轉生兩次（只算終職與加成）Lv %d／B%d；再加上保留的隊伍 Lv %d／B%d", fl, f, rl, r, rpl, rp)
	if rl <= fl || r <= f {
		t.Fatalf("只算終職與轉生加成也要升得更快、走得更深：Lv %d/B%d vs Lv %d/B%d", fl, f, rl, r)
	}
	if rp < f+f/5 {
		t.Fatalf("實際轉生（含保留的隊伍）應至少深 20%%：B%d vs B%d", f, rp)
	}
}
