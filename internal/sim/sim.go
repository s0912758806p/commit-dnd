// Package sim 是 `dnd sim` 的模擬：用正式的 engine.Apply 重播一串歷史 commit，全部在記憶體裡，
// 不讀寫 DND_HOME。時間就是 commit 的作者時間，所以每日補給、飽足下降都和真的玩一樣。
//
// 選用的地下城估計：每筆 commit 之前插入 AITurnsPerCommit 個假的 AI 回合（每回合先 EditsPerTurn 次
// Edit、最後 Stop），平均分布在這筆 commit 之前、與上一筆之間（最多 5 分鐘一個回合）。
package sim

import (
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// Targets 是升級時間表列出的等級。
var Targets = []int{5, 10, 20, 30}

// turnGap 是假的 AI 回合之間最多的間隔。
const turnGap = 5 * time.Minute

type Options struct {
	Species          string
	HatchMix         engine.Mix // 孵蛋時的 commit 組成（dnd sim --from 時是孵蛋日前 90 天的 commit）
	AITurnsPerCommit int
	EditsPerTurn     int
}

// Milestone 是第一次達成某件事的時間：Day 是從第一筆 commit 的日期起算的第幾天（當天為 1），
// Commits 是當時已重播的 commit 數。At 為零值表示沒有達成。
type Milestone struct {
	Level   int
	At      time.Time
	Day     int
	Commits int
}

func (m Milestone) Reached() bool { return !m.At.IsZero() }

type Report struct {
	Opts        Options
	Commits     int
	First, Last time.Time
	Days        int         // 第一筆到最後一筆 commit 的日曆天數（含頭尾）
	Milestones  []Milestone // 依 Targets 的順序
	Judged      Milestone   // 轉職的時間；Level 是當時的等級
	Final       engine.State
	TotalXP     int // 結束時累積的 XP（含已花在升級上的）
	// ETA 是沒到 Lv 30 時，以平均每天的 XP 推估「第幾天」會到；已經到了或無法推估時為 0。
	ETA int
}

// XPPerDay 是平均每個日曆天拿到的 XP。
func (r Report) XPPerDay() int {
	if r.Days == 0 {
		return 0
	}
	return r.TotalXP / r.Days
}

// Run 重播 commits（需已依時間排序）。沒有 commit 時回傳只有 Opts 的報告。
func Run(commits []git.LogCommit, opt Options, c *content.Pack) Report {
	r := Report{Opts: opt, Commits: len(commits)}
	for _, lv := range Targets {
		r.Milestones = append(r.Milestones, Milestone{Level: lv})
	}
	if len(commits) == 0 {
		return r
	}
	r.First, r.Last = commits[0].At, commits[len(commits)-1].At
	r.Days = day(r.First, r.Last)

	var s engine.State
	var seq int64
	done := 0
	apply := func(e event.Event) {
		seq++
		e.V, e.Seq = event.Version, seq
		s = engine.Apply(s, e, c).State
		for i := range r.Milestones {
			if m := &r.Milestones[i]; !m.Reached() && s.Pet.Level >= m.Level {
				*m = Milestone{Level: m.Level, At: e.At, Day: day(r.First, e.At), Commits: done}
			}
		}
		if !r.Judged.Reached() && s.Pet.Class != "" {
			r.Judged = Milestone{Level: s.Pet.Level, At: e.At, Day: day(r.First, e.At), Commits: done}
		}
	}

	apply(event.Event{At: r.First.Add(-time.Minute), Source: event.SourceUser, Type: "hatch",
		Args: map[string]string{"species": opt.Species, "name": "模擬", "mix": opt.HatchMix.String()}})
	for i, lc := range commits {
		if n := opt.AITurnsPerCommit; n > 0 {
			window := time.Duration(n) * turnGap
			if i > 0 {
				window = min(window, lc.At.Sub(commits[i-1].At))
			}
			for k := range n {
				at := lc.At.Add(-window + window*time.Duration(k+1)/time.Duration(n+1))
				for range opt.EditsPerTurn {
					apply(event.Event{At: at, Source: event.SourceClaude, Type: "tool_use", Tool: "Edit"})
				}
				apply(event.Event{At: at, Source: event.SourceClaude, Type: "turn"})
			}
		}
		done++
		info := lc.Commit
		apply(event.Event{At: lc.At, Source: event.SourceGit, Type: "commit", Commit: &info})
	}

	r.Final = s
	r.TotalXP = s.Pet.XP
	for lv := 1; lv < s.Pet.Level; lv++ {
		r.TotalXP += c.Cost(lv)
	}
	if last := Targets[len(Targets)-1]; s.Pet.Level < last && r.TotalXP > 0 {
		need := 0
		for lv := 1; lv < last; lv++ {
			need += c.Cost(lv)
		}
		r.ETA = (need*r.Days + r.TotalXP - 1) / r.TotalXP
	}
	return r
}

// day 回傳 t 是從 first 的日期起算的第幾天（各自用自己的當地日期）。
func day(first, t time.Time) int {
	civil := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	return int(civil(t).Sub(civil(first)).Hours()/24) + 1
}
