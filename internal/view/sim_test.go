package view

import (
	"fmt"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/sim"
)

func TestSim(t *testing.T) {
	c, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{"docs", "docs", "feat", "fix", "feat", "test", "feat", "feat", "chore", "feat"}
	var commits []git.LogCommit
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, now.Location())
	for d := range 20 {
		for i := range 6 {
			n := len(commits)
			commits = append(commits, git.LogCommit{
				At:     start.AddDate(0, 0, d).Add(time.Duration(i) * 90 * time.Minute),
				Commit: event.CommitInfo{Kind: kinds[n%len(kinds)], Added: 25, Deleted: 5, Hash: fmt.Sprintf("%04d", n)},
			})
		}
	}
	opt := sim.Options{Species: "dwarf"}
	got := Sim("demo", sim.Run(commits, opt, c), c) + "---\n"
	opt.AITurnsPerCommit, opt.EditsPerTurn = 3, 2
	opt.HatchMix.Add("refactor")
	got += Sim("demo", sim.Run(commits, opt, c), c) + "---\n"
	got += Sim("demo", sim.Run(nil, opt, c), c)
	golden(t, "sim.txt", got)
}
