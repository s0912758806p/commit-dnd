package engine

// 無盡地下城的規則（迭代 3）。由 Apply 呼叫，全部是純函式：
//
//   - AI 回合（turn）：倒地中什麼都不做；眼前沒有怪物時，這層還沒打完就遇到下一隻，打完了
//     （滿 encounters_per_floor 隻，或打倒 Boss）就往下探一層（Boss 層只有 Boss）；
//     有怪物就被攻擊扣 HP，HP 歸零即倒地並遺失所有未鑑定裝備。
//   - 攻擊工具（Edit、Write…）的 tool_use：寵物攻擊這層的怪物；打倒後掉一件未鑑定裝備，
//     打倒 Boss 則解鎖該層傳送點，並有機率掉落轉職卷軸（見 class.go）。
//   - commit：倒地中就救起來（同一層、同一隻怪物繼續），並鑑定背包裡最舊的一件，比身上的好才換上。
//   - warp：只能去入口（0）或已解鎖、不深於 Warp 的 Boss 層，倒地中不能傳送。
//   - 紮營：在 advance 裡，最後一次活動後閒置滿 camp_after_hours 就回滿 HP（倒地中不會）。
//   - 遇到怪物時記進圖鑑；收服、合體技與隊伍在 capture.go（迭代 4）。
//
// 亂數一律由 splitmix64 產生，種子取自「最後一次 commit hash、樓層、Seq」，同一份 events.jsonl 重播結果相同。
// AI 只能推進、戰鬥，不能讓寵物回血或解除倒地（DEVPLAN 的鐵律）。

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// rng 是 splitmix64：簡單、跨平台結果固定，不依賴標準庫的演算法版本。
type rng struct{ s uint64 }

func (r *rng) next() uint64 {
	r.s += 0x9e3779b97f4a7c15
	z := r.s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int { return int(r.next() % uint64(n)) }

// seedFor 從最後一次 commit hash、樓層、Seq 與用途衍生種子。
func seedFor(s State, seq int64, salt string) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d|%d|%s", s.LastCommit, s.Dungeon.Floor, seq, salt)
	return h.Sum64()
}

func pct(v, p int) int { return v * p / 100 }

// elementPct 是 attacker 屬性對 defender 屬性的傷害倍率（百分比）；任一方沒有屬性時為 100。
func elementPct(attacker, defender string, c *content.Pack) int {
	switch {
	case attacker == "" || defender == "":
		return 100
	case content.Beats[attacker] == defender:
		return c.Balance.Dungeon.ElementAdvantagePct
	case content.Beats[defender] == attacker:
		return c.Balance.Dungeon.ElementDisadvantagePct
	}
	return 100
}

func itemLabel(it Item) string { return fmt.Sprintf("%s +%d", it.Name, it.Power) }

func (r *Result) dlog(at time.Time, c *content.Pack, tmpl string, vars map[string]string) {
	d := r.State.Dungeon
	all := map[string]string{"floor": strconv.Itoa(d.Floor), "bag": strconv.Itoa(d.BagN), "bag_max": strconv.Itoa(BagSize)}
	if m, ok := c.MonsterByID(d.Monster.ID); ok {
		all["monster"], all["element"] = m.Name, content.ElementName[m.Element]
	}
	for k, v := range vars {
		all[k] = v
	}
	r.log(at, c, tmpl, all)
}

// dungeonTurn 處理一個 AI 回合。
func dungeonTurn(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	if d.Downed {
		return
	}
	if d.Monster.ID == "" {
		spawn(r, e, c)
		return
	}
	m, _ := c.MonsterByID(d.Monster.ID)
	dmg := max(1, d.Monster.Atk-PetDef(r.State, c))
	dmg = max(1, pct(dmg, elementPct(m.Element, d.Armor.Element, c)))
	p := &r.State.Pet
	p.HP = max(0, p.HP-dmg)
	if p.HP > 0 {
		return
	}
	lost := d.BagN
	d.Downed, d.Downs = true, d.Downs+1
	d.Bag, d.BagN = [BagSize]Drop{}, 0
	r.dlog(e.At, c, c.Logs.Dungeon.Downed, map[string]string{"lost": strconv.Itoa(lost)})
	r.Fx = append(r.Fx, EffectDowned)
}

// spawn 讓下一隻怪物出現：這層打完了就先往下探一層。
func spawn(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	g := c.Balance.Dungeon
	newFloor := FloorCleared(r.State, c)
	if newFloor {
		d.Floor++
		d.Cleared = 0
		d.Deepest = max(d.Deepest, d.Floor)
	}
	id := c.BossFor(d.Floor)
	if id == "" {
		pool := c.TierMonsters(d.Floor)
		rr := rng{seedFor(r.State, e.Seq, "monster")}
		id = pool[rr.intn(len(pool))]
	}
	r.State.Dex.see(id)
	m, _ := c.MonsterByID(id)
	scale := 100 + g.GrowthPerFloorPct*(d.Floor-1)
	hp, atk := pct(m.HP, scale), pct(m.Atk, scale)
	if m.Boss {
		hp, atk = pct(hp, g.BossMultiplierPct), pct(atk, g.BossMultiplierPct)
	}
	d.Monster = Monster{ID: id, HP: max(1, hp), MaxHP: max(1, hp), Atk: max(1, atk)}
	tmpl, fx := c.Logs.Dungeon.Descend, EffectDescend
	if !newFloor {
		tmpl = c.Logs.Dungeon.Encounter
	}
	if m.Boss {
		tmpl, fx = c.Logs.Dungeon.Boss, EffectBoss
	}
	r.dlog(e.At, c, tmpl, nil)
	r.Fx = append(r.Fx, fx)
}

// attack 處理一次攻擊工具呼叫。
func attack(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	if d.Downed || d.Monster.ID == "" || !c.IsAttackTool(e.Tool) {
		return
	}
	m, _ := c.MonsterByID(d.Monster.ID)
	dmg := max(1, pct(PetAtk(r.State, c), elementPct(d.Weapon.Element, m.Element, c)))
	d.Monster.HP -= dmg
	if d.Monster.HP > 0 {
		return
	}
	defeat(r, e, c, m)
}

// defeat 處理打倒眼前的怪物 m：計數、Boss 解鎖傳送點、掉落一件未鑑定裝備。攻擊與合體技共用。
func defeat(r *Result, e event.Event, c *content.Pack, m content.Monster) {
	d := &r.State.Dungeon
	d.Kills++
	d.Cleared++
	if m.Boss {
		d.Cleared = c.Balance.Dungeon.EncountersPerFloor
		d.Warp = max(d.Warp, d.Floor)
		r.dlog(e.At, c, c.Logs.Dungeon.BossKill, nil)
		dropScroll(r, e, c)
	} else {
		r.dlog(e.At, c, c.Logs.Dungeon.Kill, nil)
	}
	if d.BagN < BagSize {
		d.Bag[d.BagN] = Drop{Floor: d.Floor, Seed: seedFor(r.State, e.Seq, "drop")}
		d.BagN++
		r.dlog(e.At, c, c.Logs.Dungeon.Drop, nil)
	} else {
		r.dlog(e.At, c, c.Logs.Dungeon.BagFull, nil)
	}
	d.Monster = Monster{}
	r.Fx = append(r.Fx, EffectKill)
}

// rescue 在 commit 時把倒地的寵物救起來，留在同一層。
func rescue(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	if !d.Downed {
		return
	}
	d.Downed = false
	p := &r.State.Pet
	p.HP = max(1, pct(MaxHP(*p, c), c.Balance.Dungeon.RescueHPPct))
	r.dlog(e.At, c, c.Logs.Dungeon.Rescue, nil)
	r.Fx = append(r.Fx, EffectRescue)
}

// identifyOne 在 commit 時鑑定背包裡最舊的一件，比身上同種類的強才換上。
func identifyOne(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	if d.BagN == 0 {
		return
	}
	it := identify(d.Bag[0], c)
	copy(d.Bag[:], d.Bag[1:d.BagN])
	d.BagN--
	d.Bag[d.BagN] = Drop{}

	slot := &d.Weapon
	if it.Kind == "armor" {
		slot = &d.Armor
	}
	vars := map[string]string{"item": itemLabel(it)}
	if slot.Name == "" || it.Power > slot.Power {
		*slot = it
		r.dlog(e.At, c, c.Logs.Dungeon.IdentifyEquip, vars)
	} else {
		r.dlog(e.At, c, c.Logs.Dungeon.IdentifySkip, vars)
	}
}

// identify 用掉落物的種子決定種類、屬性、名稱與強度。
func identify(drop Drop, c *content.Pack) Item {
	rr := rng{drop.Seed}
	kind, names := "weapon", c.Items.Weapon
	if rr.intn(2) == 1 {
		kind, names = "armor", c.Items.Armor
	}
	elem := content.Elements[rr.intn(len(content.Elements))]
	list := names[elem]
	g := c.Balance.Dungeon
	return Item{
		Kind: kind, Name: list[rr.intn(len(list))], Element: elem,
		Power: g.ItemPowerBase + drop.Floor/g.ItemPowerPerFloors + rr.intn(3),
	}
}

// warp 處理傳送點。
func warp(r *Result, e event.Event, c *content.Pack) {
	d := &r.State.Dungeon
	arg := e.Args["floor"]
	deny := func(reason string) {
		r.dlog(e.At, c, c.Logs.Dungeon.WarpDenied, map[string]string{"floor": arg, "reason": reason})
	}
	n, err := strconv.Atoi(arg)
	switch {
	case err != nil || n < 0:
		deny("樓層必須是數字")
	case d.Downed:
		deny("倒地中不能傳送")
	case n == 0:
		d.Floor, d.Cleared, d.Monster = 0, 0, Monster{}
		r.dlog(e.At, c, c.Logs.Dungeon.WarpEntrance, nil)
	case n%c.Balance.Dungeon.BossEvery != 0:
		deny(fmt.Sprintf("B%d 不是傳送點", n))
	case n > d.Warp:
		deny(fmt.Sprintf("還沒打倒 B%d 的 Boss", n))
	default: // Boss 層已打過，下一回合直接往下探
		d.Floor, d.Cleared, d.Monster = n, c.Balance.Dungeon.EncountersPerFloor, Monster{}
		r.dlog(e.At, c, c.Logs.Dungeon.Warp, nil)
	}
}
