package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	gitadapter "github.com/s0912758806p/commit-dnd/internal/adapter/git"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/sim"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// `dnd sim [repo]`：重播 repo 的 git log，估計升級時間表與職業。只讀 git log，不寫 repo，
// 也不讀寫 DND_HOME（不需要先孵蛋）。
func newSimCmd() *cobra.Command {
	var opt sim.Options
	var author, from string
	cmd := &cobra.Command{
		Use:   "sim [repo]",
		Short: "用 repo 的 git log 模擬升級時間表與職業（不寫入任何資料）",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) == 1 {
				repo = args[0]
			}
			pack, err := content.Default()
			if err != nil {
				return err
			}
			if _, ok := pack.SpeciesByID(opt.Species); !ok {
				return fmt.Errorf("沒有物種 %q（%s）", opt.Species, speciesIDs(pack))
			}
			if opt.AITurnsPerCommit < 0 || opt.EditsPerTurn < 0 {
				return fmt.Errorf("--ai-turns-per-commit 與 --edits-per-turn 不可為負數")
			}
			commits, err := gitLog(repo, "", author)
			if err != nil {
				return err
			}
			if from != "" {
				day, err := time.ParseInLocation(time.DateOnly, from, time.Local)
				if err != nil {
					return fmt.Errorf("--from 要寫成 YYYY-MM-DD：%q", from)
				}
				commits, opt.HatchMix = splitAtHatch(commits, day)
			}
			printf(cmd, "%s", view.Sim(repoName(repo), sim.Run(commits, opt, pack), pack))
			return nil
		},
	}
	cmd.Flags().StringVar(&opt.Species, "species", "human", "模擬的種族 id")
	cmd.Flags().StringVar(&author, "author", "", "只重播這個作者的 commit（git log --author 的樣式，例如 email）")
	cmd.Flags().IntVar(&opt.AITurnsPerCommit, "ai-turns-per-commit", 0, "每筆 commit 前插入幾個假的 AI 回合，用來估計地下城（0 表示不估計）")
	cmd.Flags().IntVar(&opt.EditsPerTurn, "edits-per-turn", 2, "每個假的 AI 回合有幾次 Edit（寵物攻擊次數）")
	cmd.Flags().StringVar(&from, "from", "", "在這天（YYYY-MM-DD）孵蛋：前 90 天的 commit 當作孵蛋時的 commit 組成，當天起的 commit 才重播")
	return cmd
}

// splitAtHatch 模擬「在 day 孵蛋」：day 之前 mixDays 天內的 commit 變成孵蛋時的 commit 組成（同 dnd hatch），
// day 起的 commit 留下來重播。
func splitAtHatch(commits []gitadapter.LogCommit, day time.Time) ([]gitadapter.LogCommit, engine.Mix) {
	var m engine.Mix
	since := day.AddDate(0, 0, -mixDays)
	for i, lc := range commits {
		if !lc.At.Before(day) {
			return commits[i:], m
		}
		if !lc.At.Before(since) {
			m.Add(lc.Commit.Kind)
		}
	}
	return nil, m
}

// gitLog 讀 repo 的歷史 commit（不含 merge），由舊到新。
func gitLog(repo, since, author string) ([]gitadapter.LogCommit, error) {
	out, err := exec.Command("git", append([]string{"-C", repo}, gitadapter.LogArgs(since, author)...)...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("git log（%s）：%s", repo, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git log（%s）：%w", repo, err)
	}
	return gitadapter.ParseLog(out), nil
}

// mixDays 是孵蛋時往回讀幾天的 git log 來建立 commit 組成。
const mixDays = 90

// hatchMix 讀 repo 過去 mixDays 天、目前 git 使用者（user.email）的 commit，回傳 hatch 事件的 mix 參數與
// 給使用者看的說明。不是 repo、還沒有 commit 時回傳空的 mix，之後的 commit 照樣會累加。
func hatchMix(repo string, now time.Time) (arg, note string) {
	email, _ := exec.Command("git", "-C", repo, "config", "user.email").Output()
	commits, err := gitLog(repo, now.AddDate(0, 0, -mixDays).Format(time.RFC3339), strings.TrimSpace(string(email)))
	if err != nil || len(commits) == 0 {
		return "", "commit 組成會從之後的 commit 開始累積（這裡沒有讀到過去的 commit）。"
	}
	var m engine.Mix
	for _, lc := range commits {
		m.Add(lc.Commit.Kind)
	}
	return m.String(), fmt.Sprintf("從 %s 過去 %d 天的 commit 建立 commit 組成：%s。", repoName(repo), mixDays, view.Mix(m))
}

// repoName 只取資料夾名稱，和事件的 Repo 欄位一致。
func repoName(repo string) string {
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	return filepath.Base(repo)
}
