package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

func init() {
	// 收服 40% + 40% × 已損失 HP 比例；隊員攻擊 25%；合體技 200% + 50%／隊員，時間窗 30 分鐘。
	testPack.Balance.Party = content.Party{
		CaptureBasePct: 40, CaptureMissingHPPct: 40, MemberAtkPct: 25,
		ComboWindowMinutes: 30, ComboBasePct: 200, ComboPerMemberPct: 50,
	}
	testPack.Logs.Party = content.PartyLogs{
		Capture: "收服 {monster}", CaptureFail: "掙脫 {monster}", CaptureBoss: "Boss {monster} 不能收服",
		Join: "入隊 {monster} {party}/{party_max}", Full: "滿了 {monster}", Combo: "合體 {monster} {dmg}",
		Remove: "離隊 {monster} {party}/{party_max}", Denied: "隊伍拒絕：{reason}",
	}
	// 隊伍測試需要多種一般怪物；不放進 floors，不影響地城測試。
	testPack.Monsters = append(testPack.Monsters,
		content.Monster{ID: "gob", Name: "哥布林", Element: "grass", HP: 12, Atk: 8},
		content.Monster{ID: "bat", Name: "蝙蝠", Element: "fire", HP: 8, Atk: 12},
		content.Monster{ID: "shroom", Name: "菇菇", Element: "grass", HP: 14, Atk: 20},
	)
}

func partyEv(at time.Time, op, id string) event.Event {
	e := ev("party", at)
	e.Args = map[string]string{"op": op, "id": id}
	return e
}

// fixHash 找出一個讓這筆 fix: commit 收服成功（want = true）或失敗的 commit hash。
func fixHash(t *testing.T, s State, want bool) string {
	t.Helper()
	for i := range 200 {
		h := fmt.Sprintf("h%d", i)
		if got := run(s, commitEv(t0, "fix", 0, h)).State.Dex != s.Dex; got == want {
			return h
		}
	}
	t.Fatalf("找不到收服結果為 %v 的 hash", want)
	return ""
}

var slimyB2 = atFloor(2, Monster{ID: "slimy", HP: 11, MaxHP: 11, Atk: 4})

func TestCaptureSuccess(t *testing.T) {
	s := slimyB2
	s.Dungeon.Cleared = 1
	r := run(s, commitEv(t0, "fix", 0, fixHash(t, s, true)))
	d := r.State.Dungeon
	if d.Monster != (Monster{}) || d.Cleared != 2 || d.Kills != 0 || d.BagN != 0 {
		t.Fatalf("收服後怪物離開、算這層打倒一隻、不掉裝備也不算擊敗: %+v", d)
	}
	en, ok := r.State.Dex.Lookup("slimy")
	if !ok || en.Caught != 1 || en.BestFloor != 2 {
		t.Fatalf("圖鑑: %+v %v", en, ok)
	}
	if !r.State.Party.Has("slimy") || r.State.Party.N != 1 || !strings.HasSuffix(texts(r), "收服 黏黏 | 入隊 黏黏 1/3") {
		t.Fatalf("party=%+v logs=%q", r.State.Party, texts(r))
	}
}

func TestCaptureRules(t *testing.T) {
	boss := atFloor(3, Monster{ID: "king", HP: 1, MaxHP: 48, Atk: 1})
	downed := slimyB2
	downed.Dungeon.Downed, downed.Pet.HP = true, 0
	empty := slimyB2
	empty.Dungeon.Monster = Monster{}
	fail := fixHash(t, slimyB2, false)
	cases := []struct {
		name string
		s    State
		e    event.Event
		log  string // 這筆 commit 日誌的結尾
	}{
		{"失敗時寫一句日誌", slimyB2, commitEv(t0, "fix", 0, fail), "掙脫 黏黏"},
		{"Boss 不能收服", boss, commitEv(t0, "fix", 0, "a"), "Boss 國王 不能收服"},
		{"倒地中不能收服（commit 只救援）", downed, commitEv(t0, "fix", 0, "a"), "救援 B2 | 補給 | 小橘 搬了 0 行 +20 | 小橘 Lv2"},
		{"不是 fix 不收服", slimyB2, commitEv(t0, "feat", 0, "a"), "小橘 新功能 +60 | 小橘 Lv2"},
		{"眼前沒有怪物", empty, commitEv(t0, "fix", 0, "a"), "小橘 搬了 0 行 +20 | 小橘 Lv2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(tc.s, tc.e)
			want := tc.s.Dungeon.Monster
			if r.State.Dex != tc.s.Dex || r.State.Party != tc.s.Party || r.State.Dungeon.Monster != want ||
				!strings.HasSuffix(texts(r), tc.log) {
				t.Fatalf("monster=%+v dex=%d logs=%q", r.State.Dungeon.Monster, r.State.Dex.N, texts(r))
			}
		})
	}
}

// 同一個 hash 永遠同樣結果；機率大致符合 40% + 40% × 已損失 HP 比例。
func TestCaptureChanceIsDeterministic(t *testing.T) {
	for _, tc := range []struct {
		hp       int
		lo, high int
	}{{11, 30, 50}, {1, 66, 86}} {
		s := slimyB2
		s.Dungeon.Monster.HP = tc.hp
		caught := 0
		for i := range 400 {
			e := commitEv(t0, "fix", 0, fmt.Sprintf("x%d", i))
			a, b := run(s, e), run(s, e)
			if a.State != b.State || texts(a) != texts(b) {
				t.Fatalf("同樣輸入結果不同: %q vs %q", texts(a), texts(b))
			}
			if a.State.Dungeon.Monster.ID == "" {
				caught++
			}
		}
		if pct := caught * 100 / 400; pct < tc.lo || pct > tc.high {
			t.Errorf("HP %d/11 收服率 %d%%，預期 %d–%d%%", tc.hp, pct, tc.lo, tc.high)
		}
	}
}

func TestComboWindow(t *testing.T) {
	big := atFloor(2, Monster{ID: "slimy", HP: 100, MaxHP: 100, Atk: 4})
	ai := run(big, toolUse(t0, "Read")).State // Read 不攻擊，但算 AI 活動
	if !ai.Party.LastAI.Equal(t0) {
		t.Fatalf("LastAI = %v", ai.Party.LastAI)
	}
	for _, tc := range []struct {
		after time.Duration
		combo bool
	}{{0, true}, {30 * time.Minute, true}, {31 * time.Minute, false}} {
		r := run(ai, commitEv(t0.Add(tc.after), "feat", 0, "a"))
		// feat 60 XP：Lv1 → Lv2，攻擊 5 + 2×2 = 9；合體技 ×200% = 18
		hp := r.State.Dungeon.Monster.HP
		if tc.combo && (hp != 82 || r.State.Party.Combos != 1 || !strings.HasSuffix(texts(r), "合體 黏黏 18")) {
			t.Errorf("%v 後 commit 應發動合體技: hp=%d logs=%q", tc.after, hp, texts(r))
		}
		if !tc.combo && (hp != 100 || r.State.Party.Combos != 0) {
			t.Errorf("%v 後 commit 不應發動合體技: hp=%d", tc.after, hp)
		}
	}
	if s := run(big, commitEv(t0, "feat", 0, "a"), tick(t0.Add(time.Minute))).State; !s.Party.LastAI.IsZero() {
		t.Fatalf("commit 與 tick 不算 AI 活動: %v", s.Party.LastAI)
	}
}

func TestComboKillsAndScalesWithParty(t *testing.T) {
	s := run(atFloor(2, Monster{ID: "slimy", HP: 18, MaxHP: 18, Atk: 4}), toolUse(t0, "Read")).State
	r := run(s, commitEv(t0, "feat", 0, "a"))
	if d := r.State.Dungeon; d.Monster.ID != "" || d.Kills != 1 || d.Cleared != 1 || d.BagN != 1 || !strings.Contains(texts(r), "合體 黏黏 18 | 打倒 黏黏 | 掉落 1/10") {
		t.Fatalf("合體技打倒照常掉落: %+v logs=%q", d, texts(r))
	}

	// 隊員兩位：倍率 200% + 2 × 50% = 300%；攻擊另加隊伍加成
	s.Dungeon.Monster.HP, s.Dungeon.Monster.MaxHP = 500, 500
	s.Party.Members[0], s.Party.Members[1], s.Party.N = "gob", "bat", 2
	s.Dex.Entries[0], s.Dex.Entries[1], s.Dex.N = DexEntry{ID: "gob", Caught: 1, BestFloor: 1}, DexEntry{ID: "bat", Caught: 1, BestFloor: 11}, 2
	r = run(s, commitEv(t0, "feat", 0, "a"))
	// 隊伍加成：哥布林 8 × 100% × 25% = 2；蝙蝠 12 × 200%（B11）× 25% = 6。攻擊 9 + 8 = 17，× 300% = 51
	if PartyAtk(r.State, testPack) != 8 || ComboPct(r.State, testPack) != 300 || r.State.Dungeon.Monster.HP != 449 {
		t.Fatalf("party=%d combo=%d hp=%d", PartyAtk(r.State, testPack), ComboPct(r.State, testPack), r.State.Dungeon.Monster.HP)
	}
}

// 收服優先：收服成功就不發動合體技；收服失敗時合體技照常發動。Boss 也吃合體技。
func TestCaptureBeforeCombo(t *testing.T) {
	s := run(slimyB2, toolUse(t0, "Read")).State
	if r := run(s, commitEv(t0, "fix", 0, fixHash(t, slimyB2, true))); r.State.Party.Combos != 0 || r.State.Dex.N != 1 {
		t.Fatalf("收服成功不應再合體: %q", texts(r))
	}
	if r := run(s, commitEv(t0, "fix", 0, fixHash(t, slimyB2, false))); r.State.Party.Combos != 1 || !strings.HasSuffix(texts(r), "掙脫 黏黏 | 合體 黏黏 18 | 打倒 黏黏 | 掉落 1/10") {
		t.Fatalf("收服失敗後合體: %q", texts(r))
	}
	boss := run(atFloor(3, Monster{ID: "king", HP: 100, MaxHP: 100, Atk: 1}), toolUse(t0, "Read")).State
	// 測試內容沒有 fix 的基本值，算 other：20 XP → Lv2，攻擊 9 × 200% = 18
	if r := run(boss, commitEv(t0, "fix", 0, "a")); r.State.Dungeon.Monster.HP != 82 || !strings.HasSuffix(texts(r), "Boss 國王 不能收服 | 合體 國王 18") {
		t.Fatalf("Boss: hp=%d logs=%q", r.State.Dungeon.Monster.HP, texts(r))
	}
	downed := s
	downed.Dungeon.Downed, downed.Pet.HP = true, 0
	if r := run(downed, commitEv(t0, "feat", 0, "a")); r.State.Party.Combos != 0 {
		t.Fatalf("救援的那筆 commit 不發動合體技: %q", texts(r))
	}
}

func TestDexRecordsEncounters(t *testing.T) {
	s := hatched
	s.Dungeon.Floor, s.Dungeon.Cleared = 2, 2 // 下一層 B3 是 Boss 層
	s = run(s, aiTurn(t0)).State
	s.Dungeon.Monster, s.Dungeon.Cleared = Monster{}, 2
	s = run(s, aiTurn(t0)).State // B4 一般怪物
	s.Dungeon.Monster = Monster{}
	s = run(s, aiTurn(t0)).State // 同一層再遇到一次
	if en, _ := s.Dex.Lookup("king"); en.Seen != 1 || en.Caught != 0 {
		t.Fatalf("Boss 也記遇過: %+v", en)
	}
	if en, _ := s.Dex.Lookup("slimy"); en.Seen != 2 || s.Dex.N != 2 || s.Dex.Entries[0].ID != "king" {
		t.Fatalf("依第一次遇到的順序記錄: %+v", s.Dex.Entries[:s.Dex.N])
	}
}

// 自動入隊到滿 3 隻；同種不重複入隊，但最深收服樓層會更新。
func TestCaptureFillsParty(t *testing.T) {
	s := hatched
	for _, id := range []string{"slimy", "gob", "bat", "shroom", "slimy"} {
		m, _ := testPack.MonsterByID(id)
		s.Dungeon.Floor, s.Dungeon.Monster = 5, Monster{ID: id, HP: 1, MaxHP: m.HP, Atk: m.Atk}
		r := run(s, commitEv(t0, "fix", 0, fixHash(t, s, true)))
		s = r.State
		if id == "shroom" && !strings.HasSuffix(texts(r), "收服 菇菇 | 滿了 菇菇") {
			t.Fatalf("隊伍滿了: %q", texts(r))
		}
	}
	if p := s.Party; p.N != 3 || p.Members != [PartySize]string{"slimy", "gob", "bat"} {
		t.Fatalf("party=%+v", p)
	}
	if en, _ := s.Dex.Lookup("slimy"); en.Caught != 2 || en.BestFloor != 5 {
		t.Fatalf("%+v", en)
	}
	// 攻擊 = 5 + 2 × 等級 + 隊伍：黏黏 4×140%×25% = 1、哥布林 8×140%×25% = 2、蝙蝠 12×140%×25% = 4
	if got, want := PetAtk(s, testPack), 5+2*s.Pet.Level+7; PartyAtk(s, testPack) != 7 || got != want {
		t.Fatalf("PetAtk=%d want %d（隊伍 %d）", got, want, PartyAtk(s, testPack))
	}
}

func TestPartyCommand(t *testing.T) {
	s := hatched
	s.Dex.Entries[0], s.Dex.Entries[1], s.Dex.Entries[2], s.Dex.N = DexEntry{ID: "slimy", Seen: 3, Caught: 1}, DexEntry{ID: "gob", Seen: 1}, DexEntry{ID: "bat", Caught: 1}, 3
	s.Dex.Entries[3], s.Dex.Entries[4], s.Dex.N = DexEntry{ID: "shroom", Caught: 2}, DexEntry{ID: "king", Seen: 1}, 5
	s.Party.Members, s.Party.N = [PartySize]string{"slimy", "bat"}, 2
	full := s
	full.Party.Members, full.Party.N = [PartySize]string{"slimy", "bat", "shroom"}, 3
	cases := []struct {
		name string
		s    State
		e    event.Event
		want [PartySize]string
		log  string
	}{
		{"加入", s, partyEv(t0, "add", "shroom"), [PartySize]string{"slimy", "bat", "shroom"}, "入隊 菇菇 3/3"},
		{"離開並往前補", s, partyEv(t0, "remove", "slimy"), [PartySize]string{"bat"}, "離隊 黏黏 1/3"},
		{"隊伍滿了", full, partyEv(t0, "add", "shroom"), full.Party.Members, "隊伍拒絕：菇菇 已經在隊伍裡"},
		{"滿了再加", func() State { f := full; f.Dex.Entries[1].Caught = 1; return f }(), partyEv(t0, "add", "gob"), full.Party.Members, "隊伍拒絕：隊伍滿了（最多 3 隻），先讓一隻離開"},
		{"沒收服過", s, partyEv(t0, "add", "gob"), s.Party.Members, "隊伍拒絕：還沒收服過 哥布林"},
		{"Boss 沒收服過", s, partyEv(t0, "add", "king"), s.Party.Members, "隊伍拒絕：還沒收服過 國王"},
		{"不存在的怪物", s, partyEv(t0, "add", "ghost"), s.Party.Members, "隊伍拒絕：沒有 ghost 這種怪物"},
		{"不在隊伍裡", s, partyEv(t0, "remove", "shroom"), s.Party.Members, "隊伍拒絕：菇菇 不在隊伍裡"},
		{"不認得的操作", s, partyEv(t0, "swap", "slimy"), s.Party.Members, `隊伍拒絕：不認得的操作 "swap"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := run(tc.s, tc.e)
			if r.State.Party.Members != tc.want || texts(r) != tc.log || r.State.Dex != tc.s.Dex {
				t.Fatalf("party=%+v logs=%q", r.State.Party, texts(r))
			}
		})
	}
	if r := Apply(State{}, partyEv(t0, "add", "slimy"), testPack); texts(r) != "沒有寵物" {
		t.Fatalf("孵蛋前: %q", texts(r))
	}
}

// 迭代 4 的重播：同一串事件（遇怪、攻擊、收服、合體技、調整隊伍）重播兩次結果完全相同。
func TestCaptureReplayIsDeterministic(t *testing.T) {
	var events []event.Event
	at := t0
	for i := range 120 {
		at = at.Add(10 * time.Minute)
		events = append(events, toolUse(at, "Edit"), aiTurn(at))
		if i%3 == 0 {
			kind := "feat"
			if i%2 == 0 {
				kind = "fix"
			}
			events = append(events, commitEv(at.Add(time.Minute), kind, 5, fmt.Sprintf("c%d", i)))
		}
		if i == 60 {
			events = append(events, partyEv(at, "remove", "slimy"), partyEv(at, "add", "slimy"))
		}
	}
	replay := func() State {
		s := hatched
		for i, e := range events {
			e.Seq = int64(i + 2)
			s = Apply(s, e, testPack).State
		}
		return s
	}
	a, b := replay(), replay()
	if a != b {
		t.Fatal("重播結果不同")
	}
	if en, _ := a.Dex.Lookup("slimy"); en.Caught == 0 || a.Party.Combos == 0 {
		t.Fatalf("這串事件應該至少收服一次、合體一次: %+v combos=%d", en, a.Party.Combos)
	}
}

// 前期收服的弱怪物也至少 +1；member_atk_pct 為 0 時關閉隊伍加成。
func TestMemberAtkFloor(t *testing.T) {
	s := hatched
	s.Dex.Entries[0], s.Dex.N = DexEntry{ID: "slimy", Caught: 1, BestFloor: 1}, 1 // 4 × 25% = 1
	s.Party.Members[0], s.Party.N = "slimy", 1
	testPack.Monsters[0].Atk = 2 // 2 × 25% = 0 → 至少 1
	defer func() { testPack.Monsters[0].Atk = 4 }()
	if got := MemberAtk(s, testPack, "slimy"); got != 1 {
		t.Fatalf("MemberAtk = %d", got)
	}
	off := *testPack
	off.Balance.Party.MemberAtkPct = 0
	if got := PartyAtk(s, &off); got != 0 {
		t.Fatalf("關閉時 PartyAtk = %d", got)
	}
}
