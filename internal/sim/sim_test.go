package sim

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

var cst = time.FixedZone("CST", 8*3600)

// typical 產生「一般玩法」的 commit：每天 perDay 筆，9:00 起每小時一筆，
// 類型依使用者幾個 repo 的平均組成輪流（feat 45%、docs 20%、fix 15%、test 10%、其他 10%），每筆改 40 行。
func typical(days, perDay int) []git.LogCommit {
	kinds := []string{"feat", "docs", "feat", "fix", "feat", "test", "feat", "docs", "chore", "feat",
		"fix", "feat", "docs", "feat", "refactor", "feat", "test", "fix", "feat", "docs"}
	var out []git.LogCommit
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, cst)
	for d := range days {
		for i := range perDay {
			n := len(out)
			out = append(out, git.LogCommit{
				At:     start.AddDate(0, 0, d).Add(time.Duration(i) * time.Hour),
				Commit: event.CommitInfo{Kind: kinds[n%len(kinds)], Added: 30, Deleted: 10, Hash: fmt.Sprintf("h%05d", n)},
			})
		}
	}
	return out
}

func pack(t *testing.T) *content.Pack {
	t.Helper()
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// DEVPLAN 迭代 5 的驗收：一般玩法到 Lv 30 落在 1 到 2 個月。DESIGN-NOTES 第 46 條寫的天數綁住實算值。
func TestTypicalPlayReachesLv30InOneToTwoMonths(t *testing.T) {
	c := pack(t)
	notes, err := os.ReadFile("../../docs/DESIGN-NOTES.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, perDay := range []int{5, 8, 12} {
		r := Run(typical(90, perDay), Options{Species: "human"}, c)
		lv30 := r.Milestones[len(r.Milestones)-1]
		claim := fmt.Sprintf("每天 %d 筆：第 %d 天到 Lv 30", perDay, lv30.Day)
		t.Log(claim, "；Lv 5／10／20：", r.Milestones[0].Day, r.Milestones[1].Day, r.Milestones[2].Day)
		if !strings.Contains(string(notes), claim) {
			t.Errorf("DESIGN-NOTES.md 沒有寫「%s」（數值改了就要同步文件）", claim)
		}
		if perDay == 8 && (!lv30.Reached() || lv30.Day < 30 || lv30.Day > 62) {
			t.Errorf("每天 8 筆到 Lv 30 要在 1–2 個月內，實際 %+v", lv30)
		}
	}
}

func TestRun(t *testing.T) {
	c := pack(t)
	commits := typical(3, 4)
	r := Run(commits, Options{Species: "elf"}, c)
	if r.Commits != 12 || r.Days != 3 || !r.First.Equal(commits[0].At) || !r.Last.Equal(commits[11].At) {
		t.Fatalf("%+v", r)
	}
	m := r.Milestones
	if !m[0].Reached() || m[0].Level != 5 || m[1].Reached() || r.ETA <= r.Days {
		t.Fatalf("milestones=%+v eta=%d", m, r.ETA)
	}
	if r.Judged.Level != 5 || r.Judged.Day != m[0].Day || r.Final.Pet.Class == "" || r.Final.Pet.Mix.Total() != 12 {
		t.Fatalf("judged=%+v pet=%+v", r.Judged, r.Final.Pet)
	}
	if r.Final.AI.Turns != 0 || r.Final.Dungeon.Deepest != 0 {
		t.Fatal("沒有指定 AI 回合時不碰地下城")
	}
	total := r.Final.Pet.XP
	for lv := 1; lv < r.Final.Pet.Level; lv++ {
		total += c.Cost(lv)
	}
	if r.TotalXP != total || r.XPPerDay() != total/3 {
		t.Fatalf("TotalXP=%d want %d", r.TotalXP, total)
	}
	if again := Run(commits, Options{Species: "elf"}, c); again.Final != r.Final {
		t.Fatal("同樣的輸入要得到同樣結果")
	}
	if empty := Run(nil, Options{Species: "elf"}, c); empty.Commits != 0 || empty.Milestones[0].Reached() {
		t.Fatalf("%+v", empty)
	}
}

func TestAITurns(t *testing.T) {
	c := pack(t)
	commits := typical(2, 3)
	r := Run(commits, Options{Species: "human", AITurnsPerCommit: 4, EditsPerTurn: 2}, c)
	if ai := r.Final.AI; ai.Turns != 24 || ai.ToolUses != 48 {
		t.Fatalf("%+v", ai)
	}
	if d := r.Final.Dungeon; d.Deepest == 0 || d.Kills == 0 {
		t.Fatalf("應該有下探與戰鬥: %+v", d)
	}
	// AI 回合不影響經驗（經驗只來自 commit）
	if plain := Run(commits, Options{Species: "human"}, c); plain.TotalXP != r.TotalXP {
		t.Fatalf("XP %d != %d", plain.TotalXP, r.TotalXP)
	}
	// 孵蛋時的組成會算進最後的組成
	withMix := Options{Species: "human"}
	withMix.HatchMix.Add("test")
	if got := Run(commits, withMix, c).Final.Pet.Mix.Total(); got != 7 {
		t.Fatalf("mix total = %d", got)
	}
}

func TestDay(t *testing.T) {
	first := time.Date(2026, 10, 1, 23, 0, 0, 0, cst)
	for at, want := range map[time.Time]int{
		first: 1, first.Add(30 * time.Minute): 1, first.Add(2 * time.Hour): 2, first.AddDate(0, 1, 0): 32,
	} {
		if got := day(first, at); got != want {
			t.Fatalf("day(%v) = %d, want %d", at, got, want)
		}
	}
}
