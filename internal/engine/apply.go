// Package engine 是遊戲規則本身：Apply(state, event, content) → 新狀態、日誌、動畫提示。
//
// Apply 是純函式：不讀時鐘、不碰檔案、不用全域亂數，同樣輸入永遠同樣輸出，
// 這是重播、模擬與 Rebuild 的基礎。每次 Apply 先依 e.At − s.UpdatedAt 推進時間效果
// （Δ < 0 時視為 0、UpdatedAt 只前進；孵蛋前沒有時間效果），再依 e.Type 分派：
//
//   - hatch：Args 的 species／name 孵出寵物；已有寵物或物種不存在時不變
//   - commit：依類型與行數給 XP；當地每天前幾筆乘上補給倍率，飽足過低減半；同時餵食、回血、升級。
//     與上一筆計分的 commit 同 hash 時不計分（hook 重跑、重複安裝）
//   - pet：心情上升
//   - turn／tool_use：計數 AI 回合與工具呼叫（孵蛋前也計數）；孵蛋後推進地下城（見 dungeon.go）
//   - warp：使用者的傳送指令（見 dungeon.go）
//   - debug_xp／debug_floor：`dnd debug xp`／`dnd debug floor` 寫入的除錯事件（見 debug.go）
//   - 其他類型（含 tick）：只推進時間
//
// commit 會先救起倒地的寵物、最後鑑定一件未鑑定裝備。commit、turn、tool_use 算「活動」，
// 會更新 Dungeon.LastActive。
//
// 時間效果（飽足、心情下降，HP 回復，閒置滿 camp_after_hours 紮營回滿）必須「路徑無關」：
// 一次推進 Δ1+Δ2 與分兩次結果相同（見 TestTimeEffectsArePathIndependent），
// 因為 status 套用的 tick 不會寫入 events.jsonl。倒地中 HP 不會自然回復，也不會紮營。
package engine

import (
	"hash/fnv"
	"math/bits"
	"strconv"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

type Result struct {
	State State
	Logs  []LogLine
	Fx    []Effect
}

func (r *Result) log(at time.Time, c *content.Pack, tmpl string, vars map[string]string) {
	p := r.State.Pet
	pairs := []string{"{name}", p.Name, "{level}", strconv.Itoa(p.Level)}
	if sp, ok := c.SpeciesByID(p.Species); ok {
		pairs = append(pairs, "{species}", sp.Name)
	}
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	r.Logs = append(r.Logs, LogLine{At: at, Text: strings.NewReplacer(pairs...).Replace(tmpl)})
}

func Apply(s State, e event.Event, c *content.Pack) Result {
	s.V = StateVersion
	s.ContentHash = c.Hash
	s, camped := advance(s, e.At, c)
	if e.Seq > s.LastSeq {
		s.LastSeq = e.Seq
	}

	r := Result{State: s}
	if camped {
		r.dlog(e.At, c, c.Logs.Dungeon.Camp, nil)
		r.Fx = append(r.Fx, EffectCamp)
	}
	switch e.Type {
	case "hatch":
		hatch(&r, e, c)
	case "commit":
		commit(&r, e, c)
	case "pet":
		pet(&r, e, c)
	case "turn":
		ai := &r.State.AI
		if day := e.At.Format(time.DateOnly); day != ai.Day {
			ai.Day, ai.TodayTurns = day, 0
		}
		ai.Turns++
		ai.TodayTurns++
		if r.State.Pet.Hatched() {
			dungeonTurn(&r, e, c)
		}
	case "tool_use":
		r.State.AI.ToolUses++
		if r.State.Pet.Hatched() {
			attack(&r, e, c)
		}
	case "warp":
		if r.State.Pet.Hatched() {
			warp(&r, e, c)
		} else {
			r.log(e.At, c, c.Logs.NoPet, nil)
		}
	case "debug_xp", "debug_floor":
		debugEvent(&r, e, c)
	}
	switch e.Type {
	case "commit", "turn", "tool_use":
		if d := &r.State.Dungeon; e.At.After(d.LastActive) {
			d.LastActive = e.At
		}
	}
	return r
}

// advance 推進時間效果。未滿一個週期的時間留在各自的 Carry，所以切分方式不影響結果。
// camped 表示這次推進觸發了紮營（HP 回滿）。
func advance(s State, at time.Time, c *content.Pack) (_ State, camped bool) {
	var elapsed time.Duration
	if !s.UpdatedAt.IsZero() && at.After(s.UpdatedAt) {
		elapsed = at.Sub(s.UpdatedAt)
	}
	if s.UpdatedAt.IsZero() || at.After(s.UpdatedAt) {
		s.UpdatedAt = at
	}
	p := &s.Pet
	if !p.Hatched() {
		return s, false
	}

	b := c.Balance
	n := periods(&p.SatietyCarry, elapsed, b.Satiety.DecayEveryMinutes)
	p.Satiety = drain(p.Satiety, n, b.Satiety.DecayAmount)
	n = periods(&p.MoodCarry, elapsed, b.Mood.DecayEveryMinutes)
	p.Mood = drain(p.Mood, n, b.Mood.DecayAmount)
	n = periods(&p.HPCarry, elapsed, b.HP.RegenEveryMinutes)
	d := s.Dungeon
	if d.Downed {
		return s, false
	}
	maxHP := c.MaxHP(p.Level)
	p.HP = fill(p.HP, n, b.HP.RegenAmount, maxHP)
	idle := time.Duration(b.Dungeon.CampAfterHours) * time.Hour
	if !d.LastActive.IsZero() && at.Sub(d.LastActive) >= idle && p.HP < maxHP {
		p.HP = maxHP
		return s, true
	}
	return s, false
}

// periods 把 elapsed 加進 carry，回傳滿了幾個週期，餘數留在 carry。
func periods(carry *time.Duration, elapsed time.Duration, everyMinutes int) int64 {
	period := time.Duration(everyMinutes) * time.Minute
	total := *carry + elapsed
	*carry = total % period
	return int64(total / period)
}

// drain 扣 n 次 amount，夾在 0；n 大到足以歸零時不做乘法，避免溢位。
func drain(v int, n int64, amount int) int {
	if n > int64(v) {
		n = int64(v)
	}
	return max(0, v-int(n)*amount)
}

// fill 加 n 次 amount，夾在 limit。
func fill(v int, n int64, amount, limit int) int {
	if n > int64(limit) {
		n = int64(limit)
	}
	return min(limit, v+int(n)*amount)
}

func hatch(r *Result, e event.Event, c *content.Pack) {
	if r.State.Pet.Hatched() {
		r.log(e.At, c, c.Logs.AlreadyHatched, nil)
		return
	}
	sp, ok := c.SpeciesByID(e.Args["species"])
	if !ok {
		return // CLI 會先擋；這裡只保證壞事件不會孵出不存在的物種
	}
	name := e.Args["name"]
	if name == "" {
		name = sp.Name
	}
	b := c.Balance
	r.State.Pet = Pet{
		Species: sp.ID, Name: name, Stage: 1, Level: 1, HP: c.MaxHP(1),
		Satiety: b.Satiety.Start, Mood: b.Mood.Start,
	}
	r.log(e.At, c, c.Logs.Hatch, nil)
	r.Fx = append(r.Fx, EffectHatch)
}

func commit(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	if !p.Hatched() {
		r.log(e.At, c, c.Logs.NoPet, nil)
		return
	}
	if e.Commit == nil || (e.Commit.Hash != "" && e.Commit.Hash == r.State.LastCommit) {
		return
	}
	r.State.LastCommit = e.Commit.Hash
	rescue(r, e, c)
	b := c.Balance
	ci := e.Commit
	kind := ci.Kind
	base, ok := b.Commit.Base[kind]
	if !ok {
		kind, base = "other", b.Commit.Base["other"]
	}
	lines := max(0, ci.Added) + max(0, ci.Deleted)
	xp := base + min(b.Commit.LineBonusMax, b.Commit.LineBonusPerBit*bits.Len(uint(lines)))

	if day := e.At.Format(time.DateOnly); day != p.SupplyDay {
		p.SupplyDay, p.SupplyUsed = day, 0
	}
	if p.SupplyUsed < b.Supply.CommitsPerDay {
		p.SupplyUsed++
		xp *= b.Supply.Multiplier
		r.log(e.At, c, c.Logs.Supply, nil)
		r.Fx = append(r.Fx, EffectSupply)
	}
	if p.Satiety < b.Satiety.HungryBelow {
		xp /= 2
		r.log(e.At, c, c.Logs.Hungry, nil)
		r.Fx = append(r.Fx, EffectHungry)
	}

	p.Commits++
	p.Satiety = min(b.Satiety.Max, p.Satiety+b.Commit.Feed)
	p.HP = min(c.MaxHP(p.Level), p.HP+b.Commit.Heal)
	p.XP += xp

	tmpls, ok := c.Logs.Commit[kind]
	if !ok {
		tmpls = c.Logs.Commit["other"]
	}
	r.log(e.At, c, pick(tmpls, ci.Hash, e.Seq), map[string]string{"xp": strconv.Itoa(xp), "lines": strconv.Itoa(lines)})

	levelUp(r, e, c)
	identifyOne(r, e, c)
}

// levelUp 在 XP 足夠時連續升級，每升一級 HP 回滿。commit 與 debug_xp 共用。
func levelUp(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	for cost := c.Cost(p.Level); cost > 0 && p.XP >= cost; cost = c.Cost(p.Level) {
		p.XP -= cost
		p.Level++
		p.HP = c.MaxHP(p.Level)
		r.log(e.At, c, c.Logs.Levelup, nil)
		r.Fx = append(r.Fx, EffectLevelUp)
	}
}

func pet(r *Result, e event.Event, c *content.Pack) {
	p := &r.State.Pet
	if !p.Hatched() {
		r.log(e.At, c, c.Logs.NoPet, nil)
		return
	}
	p.Mood = min(c.Balance.Mood.Max, p.Mood+c.Balance.Mood.PerPet)
	p.Pets++
	r.log(e.At, c, c.Logs.Pet, nil)
	r.Fx = append(r.Fx, EffectHappy)
}

// pick 用 commit hash（沒有時用 Seq）決定挑哪一句，重播結果不變。
func pick(tmpls []string, hash string, seq int64) string {
	h := fnv.New32a()
	if hash != "" {
		h.Write([]byte(hash))
	} else {
		h.Write([]byte(strconv.FormatInt(seq, 10)))
	}
	return tmpls[h.Sum32()%uint32(len(tmpls))]
}
