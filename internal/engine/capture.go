package engine

// 收服、合體技、圖鑑與隊伍（迭代 4）。由 Apply 與 commit 呼叫，全部是純函式：
//
//   - 收服：fix: commit 時，眼前有一般怪物（Boss 不行）且這筆 commit 之前沒有倒地，就依機率收服它：
//     capture_base_pct + capture_missing_hp_pct × 怪物已損失 HP 的比例。成功時怪物離開戰鬥、
//     算這層打倒一隻（不掉裝備、不算擊敗數），記進圖鑑，隊伍有空位且沒有同種隊員時自動入隊。
//   - 合體技：commit 前 combo_window_minutes 分鐘內有 AI 活動（turn／tool_use，記在 Party.LastAI），
//     這筆 commit 對眼前的怪物（含 Boss）造成寵物攻擊 × ComboPct 的傷害，打倒照常掉落。
//     收服優先：同一筆 commit 收服成功就不再發動合體技；倒地中被救起的那筆 commit 兩者都不發動。
//     合體技是 commit 的效果，AI 仍然不能單獨回血或救倒地（DEVPLAN 的鐵律）。
//   - 圖鑑：遇到怪物時記「遇過」，收服時記「收服」次數與最深的收服樓層。
//   - 隊伍：最多 PartySize 隻、同種只能一隻；每位隊員讓寵物攻擊增加 MemberAtk。
//   - party 事件（dnd party add／remove）：引擎驗證，不合法時寫一句日誌、狀態不變。
//
// 亂數同 dungeon.go，用 seedFor（最後一次 commit hash、樓層、Seq、用途），重播結果不變。

import (
	"fmt"
	"strconv"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// PartySize 是隊伍的上限。State 用固定大小陣列存隊員，所以寫在引擎而不是 balance.yaml。
const PartySize = 3

const (
	EffectCapture Effect = "capture"
	EffectCombo   Effect = "combo"
)

// DexEntry 是圖鑑裡的一種怪物。
type DexEntry struct {
	ID        string `json:"id"`
	Seen      int    `json:"seen"`       // 遇過幾次
	Caught    int    `json:"caught"`     // 收服過幾次
	BestFloor int    `json:"best_floor"` // 最深在哪一層收服過；隊員的強度依此計算
}

// Dex 是圖鑑：依第一次遇到的順序記錄，前 N 個有效。用 id 當鍵而不是 monsters.yaml 裡的位置，
// 之後新增或重排怪物都不影響舊存檔；容量是 content.MaxMonsters（載入內容時驗證）。
type Dex struct {
	Entries [content.MaxMonsters]DexEntry `json:"entries"`
	N       int                           `json:"n"`
}

// Lookup 回傳某種怪物的圖鑑紀錄；沒遇過時回傳 false。
func (d Dex) Lookup(id string) (DexEntry, bool) {
	for _, en := range d.Entries[:d.N] {
		if en.ID == id {
			return en, true
		}
	}
	return DexEntry{}, false
}

// entry 回傳某種怪物的紀錄，沒有就新增；圖鑑滿了時回傳 nil。
func (d *Dex) entry(id string) *DexEntry {
	for i := range d.Entries[:d.N] {
		if d.Entries[i].ID == id {
			return &d.Entries[i]
		}
	}
	if d.N == len(d.Entries) {
		return nil
	}
	d.Entries[d.N] = DexEntry{ID: id}
	d.N++
	return &d.Entries[d.N-1]
}

// see 記錄遇到一隻怪物。
func (d *Dex) see(id string) {
	if en := d.entry(id); en != nil {
		en.Seen++
	}
}

// Party 是隊伍與合體技的狀態。
type Party struct {
	Members [PartySize]string `json:"members"` // 隊員的怪物 id，前 N 個有效（入隊順序）
	N       int               `json:"n"`
	// LastAI 是最後一次 AI 活動（turn、tool_use）的時間，只會前進；合體技的時間窗看它。
	LastAI time.Time `json:"last_ai"`
	Combos int       `json:"combos"` // 發動過幾次合體技
}

// Has 回報某種怪物是否在隊伍裡。
func (p Party) Has(id string) bool {
	for _, m := range p.Members[:p.N] {
		if m == id {
			return true
		}
	}
	return false
}

func (p *Party) add(id string) {
	p.Members[p.N] = id
	p.N++
}

func (p *Party) remove(id string) {
	for i, m := range p.Members[:p.N] {
		if m == id {
			copy(p.Members[i:], p.Members[i+1:p.N])
			p.N--
			p.Members[p.N] = ""
			return
		}
	}
}

// MemberAtk 是一位隊員給寵物的攻擊加成：該怪物在最深收服樓層的攻擊力 × member_atk_pct，至少 1
// （member_atk_pct 為 0 時關閉）。內容裡找不到這個 id 時為 0。
func MemberAtk(s State, c *content.Pack, id string) int {
	m, ok := c.MonsterByID(id)
	if !ok || c.Balance.Party.MemberAtkPct == 0 {
		return 0
	}
	en, _ := s.Dex.Lookup(id)
	scale := 100 + c.Balance.Dungeon.GrowthPerFloorPct*(max(1, en.BestFloor)-1)
	return max(1, pct(pct(m.Atk, scale), c.Balance.Party.MemberAtkPct))
}

// PartyAtk 是整個隊伍給寵物的攻擊加成。
func PartyAtk(s State, c *content.Pack) int {
	total := 0
	for _, id := range s.Party.Members[:s.Party.N] {
		total += MemberAtk(s, c, id)
	}
	return pct(total, 100+Ability(s.Pet, c, "party_pct"))
}

// ComboPct 是合體技的傷害倍率（寵物攻擊的百分比）：隊員越多越強。
func ComboPct(s State, c *content.Pack) int {
	b := c.Balance.Party
	return b.ComboBasePct + b.ComboPerMemberPct*s.Party.N + Ability(s.Pet, c, "combo_pct")
}

// ComboReady 回報在 at 這個時間 commit 是否會發動合體技（前 combo_window_minutes 內有 AI 活動）。
func ComboReady(s State, at time.Time, c *content.Pack) bool {
	window := time.Duration(c.Balance.Party.ComboWindowMinutes) * time.Minute
	return !s.Party.LastAI.IsZero() && at.Sub(s.Party.LastAI) <= window
}

// captureChance 是收服眼前怪物的機率（百分比，最多 100）；含 capture_pct 終職能力。
func captureChance(s State, c *content.Pack) int {
	b := c.Balance.Party
	m := s.Dungeon.Monster
	missing := max(0, m.MaxHP-m.HP)
	return min(100, b.CaptureBasePct+b.CaptureMissingHPPct*missing/max(1, m.MaxHP)+Ability(s.Pet, c, "capture_pct"))
}

// noteAI 記錄 AI 活動的時間。
func noteAI(r *Result, e event.Event) {
	if (e.Type == "turn" || e.Type == "tool_use") && e.At.After(r.State.Party.LastAI) {
		r.State.Party.LastAI = e.At
	}
}

// afterCommit 在 commit 計分與鑑定之後處理收服與合體技；downed 是這筆 commit 救援之前是否倒地。
func afterCommit(r *Result, e event.Event, c *content.Pack, downed bool) {
	if downed || r.State.Dungeon.Monster.ID == "" {
		return
	}
	if e.Commit.Kind == "fix" && capture(r, e, c) {
		return
	}
	if ComboReady(r.State, e.At, c) {
		combo(r, e, c)
	}
}

// plog 寫一句隊伍相關的日誌：{monster} 是指定的怪物，另有 {party}／{party_max}。
func (r *Result) plog(at time.Time, c *content.Pack, tmpl, id string, vars map[string]string) {
	name := id
	if m, ok := c.MonsterByID(id); ok {
		name = m.Name
	}
	all := map[string]string{"monster": name, "party": strconv.Itoa(r.State.Party.N), "party_max": strconv.Itoa(PartySize)}
	for k, v := range vars {
		all[k] = v
	}
	r.dlog(at, c, tmpl, all)
}

// capture 嘗試收服眼前的怪物，回報是否成功。
func capture(r *Result, e event.Event, c *content.Pack) bool {
	d := &r.State.Dungeon
	m, ok := c.MonsterByID(d.Monster.ID)
	if !ok {
		return false
	}
	if m.Boss {
		r.dlog(e.At, c, c.Logs.Party.CaptureBoss, nil)
		return false
	}
	rr := rng{seedFor(r.State, e.Seq, "capture")}
	if rr.intn(100) >= captureChance(r.State, c) {
		r.dlog(e.At, c, c.Logs.Party.CaptureFail, nil)
		return false
	}
	if en := r.State.Dex.entry(m.ID); en != nil {
		en.Caught++
		en.BestFloor = max(en.BestFloor, d.Floor)
	}
	r.dlog(e.At, c, c.Logs.Party.Capture, nil)
	r.Fx = append(r.Fx, EffectCapture)
	d.Cleared++
	d.Monster = Monster{}

	p := &r.State.Party
	switch {
	case p.Has(m.ID): // 同種已在隊伍裡：最深收服樓層更新後，這位隊員自動變強
	case p.N < PartySize:
		p.add(m.ID)
		r.plog(e.At, c, c.Logs.Party.Join, m.ID, nil)
	default:
		r.plog(e.At, c, c.Logs.Party.Full, m.ID, nil)
	}
	return true
}

// combo 發動合體技。
func combo(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	m, _ := c.MonsterByID(d.Monster.ID)
	dmg := max(1, pct(pct(PetAtk(r.State, c), ComboPct(r.State, c)), petElementPct(r.State, m, c)))
	r.State.Party.Combos++
	r.dlog(e.At, c, c.Logs.Party.Combo, map[string]string{"dmg": strconv.Itoa(dmg)})
	r.Fx = append(r.Fx, EffectCombo)
	d.Monster.HP -= dmg
	if d.Monster.HP <= 0 {
		defeat(r, e, c, m)
	}
}

// partyCmd 處理 dnd party add／remove 寫入的 party 事件。
func partyCmd(r *Result, e event.Event, c *content.Pack) {
	op, id := e.Args["op"], e.Args["id"]
	p := &r.State.Party
	name := id
	m, known := c.MonsterByID(id)
	if known {
		name = m.Name
	}
	deny := func(format string, a ...any) {
		r.dlog(e.At, c, c.Logs.Party.Denied, map[string]string{"reason": fmt.Sprintf(format, a...)})
	}
	en, _ := r.State.Dex.Lookup(id)
	switch {
	case op == "add" && !known:
		deny("沒有 %s 這種怪物", id)
	case op == "add" && en.Caught == 0:
		deny("還沒收服過 %s", name)
	case op == "add" && p.Has(id):
		deny("%s 已經在隊伍裡", name)
	case op == "add" && p.N >= PartySize:
		deny("隊伍滿了（最多 %d 隻），先讓一隻離開", PartySize)
	case op == "add":
		p.add(id)
		r.plog(e.At, c, c.Logs.Party.Join, id, nil)
	case op == "remove" && !p.Has(id):
		deny("%s 不在隊伍裡", name)
	case op == "remove":
		p.remove(id)
		r.plog(e.At, c, c.Logs.Party.Remove, id, nil)
	default:
		deny("不認得的操作 %q", op)
	}
}
