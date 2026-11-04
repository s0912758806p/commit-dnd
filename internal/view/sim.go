package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/sim"
)

// Sim 是 `dnd sim` 的輸出：升級時間表、職業判定，以及有插入 AI 回合時的地下城估計。
func Sim(repo string, r sim.Report, c *content.Pack) string {
	if r.Commits == 0 {
		return fmt.Sprintf("%s 沒有可以重播的 commit。\n", repo)
	}
	var b strings.Builder
	row := func(label, format string, a ...any) {
		fmt.Fprintf(&b, "%s  %s\n", label, fmt.Sprintf(format, a...))
	}
	date := func(t time.Time) string { return t.Format(time.DateOnly) }
	sp, _ := c.SpeciesByID(r.Opts.Species)
	p := r.Final.Pet

	fmt.Fprintf(&b, "模擬 %s：%d 筆 commit，%s ～ %s（%d 天），種族 %s\n", repo, r.Commits, date(r.First), date(r.Last), r.Days, sp.Name)
	b.WriteString("（只在記憶體重播，不寫入任何資料；時間用 commit 的作者時間）\n")
	if r.Opts.HatchMix.Total() > 0 {
		fmt.Fprintf(&b, "孵蛋時的 commit 組成：%s（孵蛋日前 90 天）\n", Mix(r.Opts.HatchMix))
	}
	b.WriteString("\n")

	b.WriteString("升級時間表\n")
	for _, m := range r.Milestones {
		label := fmt.Sprintf("  Lv %-2d", m.Level)
		switch {
		case m.Reached():
			row(label, "第 %d 天（%s），第 %d 筆 commit", m.Day, date(m.At), m.Commits)
		case m.Level == sim.Targets[len(sim.Targets)-1] && r.ETA > 0:
			row(label, "未達到（以平均每天 %d XP 推估，約第 %d 天）", r.XPPerDay(), r.ETA)
		default:
			row(label, "未達到")
		}
	}
	if cost := c.Cost(p.Level); cost > 0 {
		row("結束時", "Lv %d（XP %d/%d），%s", p.Level, p.XP, cost, stageName(p, c))
	} else {
		row("結束時", "Lv %d，%s", p.Level, stageName(p, c))
	}
	row("平均", "每天 %d XP（共 %d XP）", r.XPPerDay(), r.TotalXP)

	b.WriteString("\n職業\n")
	if cl, ok := c.ClassByID(p.Class); ok {
		row("  判定", "Lv %d 轉職為%s（%s），第 %d 天", r.Judged.Level, cl.Name, cl.Style, r.Judged.Day)
	} else {
		row("  判定", "還沒到 Lv %d，尚未轉職", c.Growth.JudgeLevel)
	}
	row("  組成", "%s", Mix(p.Mix))
	if j, ok := engine.Judge(p.Mix, c); ok {
		row("  重判", "依結束時的組成會是%s（%s）", j.Name, j.Style)
	}

	if r.Opts.AITurnsPerCommit > 0 {
		d := r.Final.Dungeon
		fmt.Fprintf(&b, "\n地下城（每筆 commit 前 %d 個 AI 回合、每回合 %d 次編輯）\n", r.Opts.AITurnsPerCommit, r.Opts.EditsPerTurn)
		if d.Warp > 0 {
			row("  最深", "%s（傳送點 %s）", floorName(d.Deepest), floorName(d.Warp))
		} else {
			row("  最深", "%s", floorName(d.Deepest))
		}
		row("  戰績", "擊敗 %d、倒地 %d", d.Kills, d.Downs)
		row("  卷軸", "轉職卷軸 %d 張", p.Scrolls)
		row("  能力", "攻擊 %d、防禦 %d、HP 上限 %d；武器 %s、防具 %s", engine.PetAtk(r.Final, c), engine.PetDef(r.Final, c), engine.MaxHP(p, c), item(d.Weapon), item(d.Armor))
	}
	return b.String()
}
