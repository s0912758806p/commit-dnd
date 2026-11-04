package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitAt 用指定的作者時間 commit（RFC 3339），給 sim 與孵蛋讀 git log 用。
func commitAt(t *testing.T, repo, msg, at string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(msg+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", msg}} {
		c := exec.Command("git", append([]string{"-C", repo}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestHatchReadsCommitMix(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	repo := gitRepo(t)
	for _, msg := range []string{"test: a", "test: b", "feat: c", "chore: d"} {
		commit(t, repo, msg)
	}
	out, err := dnd(t, "hatch", "--species", "dwarf", "--name", "鐵鎚", "--repo", repo)
	if err != nil || !strings.Contains(out, "過去 90 天的 commit 建立 commit 組成：test 2（50%）、feat 1（25%）、other 1（25%），共 4 筆") {
		t.Fatalf("%v\n%s", err, out)
	}
	out, err = dnd(t, "class")
	if err != nil || !strings.Contains(out, "組成  test 2（50%）") || !strings.Contains(out, "判定  依目前的組成會是騎士（守護型）") {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "職業  未轉職（Lv 5 依 commit 組成判定）") || !strings.Contains(out, "職業一覽") {
		t.Fatalf("還沒轉職時應列出職業一覽:\n%s", out)
	}
}

// 不是 repo（或沒有 commit）時照樣孵蛋，組成從之後的 commit 開始累積。
func TestHatchWithoutGitHistory(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	out, err := dnd(t, "hatch", "--species", "elf", "--repo", t.TempDir())
	if err != nil || !strings.Contains(out, "commit 組成會從之後的 commit 開始累積") {
		t.Fatalf("%v\n%s", err, out)
	}
}

// choose／reroll 的參數由 CLI 檢查；合不合法由引擎判斷，不合法時印出一句日誌、成功結束。
func TestClassCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	if out, err := dnd(t, "class"); err != nil || out != "還沒有寵物。用 dnd hatch 孵一隻吧。\n" {
		t.Fatalf("%v %q", err, out)
	}
	for _, args := range [][]string{{"class", "choose"}, {"class", "choose", "a", "b"}, {"class", "reroll", "x"}, {"class", "x"}} {
		if _, err := dnd(t, args...); err == nil {
			t.Fatalf("%v 應報錯", args)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("參數錯誤時不應寫入事件: %v", err)
	}
	if _, err := dnd(t, "hatch", "--species", "elf", "--repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if out, err := dnd(t, "class", "choose", "paladin"); err != nil || out != "無法選擇特化「paladin」：還沒轉職（Lv 5 依 commit 組成判定）。\n" {
		t.Fatalf("%v %q", err, out)
	}
	if out, err := dnd(t, "class", "reroll"); err != nil || out != "無法使用轉職卷軸：還沒轉職（Lv 5 依 commit 組成判定）。\n" {
		t.Fatalf("%v %q", err, out)
	}
	if out, _ := dnd(t, "status"); !strings.Contains(out, "職業  未轉職\n進化  第 1 階段（幼精靈）\n") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestSim(t *testing.T) {
	home := filepath.Join(t.TempDir(), "dnd")
	t.Setenv("DND_HOME", home)
	repo := gitRepo(t)
	// 兩天、每天 4 筆；第 1 天到 Lv 4，第 2 天的第 2 筆到 Lv 5
	for _, c := range []struct{ msg, at string }{
		{"docs: plan", "2026-10-01T09:00:00+08:00"}, {"docs: more", "2026-10-01T10:00:00+08:00"},
		{"feat: a", "2026-10-01T11:00:00+08:00"}, {"fix: b", "2026-10-01T15:00:00+08:00"},
		{"feat: c", "2026-10-02T09:00:00+08:00"}, {"feat: d", "2026-10-02T10:00:00+08:00"},
		{"feat: e", "2026-10-02T11:00:00+08:00"}, {"test: f", "2026-10-02T23:30:00+08:00"},
	} {
		commitAt(t, repo, c.msg, c.at)
	}
	out, err := dnd(t, "sim", repo, "--ai-turns-per-commit", "3")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{
		"8 筆 commit，2026-10-01 ～ 2026-10-02（2 天），種族 人族",
		"Lv 5   第 2 天（2026-10-02），第 6 筆 commit",
		"Lv 30  未達到（以平均每天",
		"判定  Lv 5 轉職為遊俠（開拓型），第 2 天",
		"組成  feat 4（50%）、docs 2（25%）、fix 1（12%）、test 1（12%），共 8 筆",
		"地下城（每筆 commit 前 3 個 AI 回合、每回合 2 次編輯）",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("缺少 %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("sim 不可建立或寫入 DND_HOME: %v", err)
	}
	// --from：之前的 commit 變成孵蛋時的組成，當天起的才重播
	out, err = dnd(t, "sim", repo, "--from", "2026-10-02")
	if err != nil || !strings.Contains(out, "4 筆 commit，2026-10-02 ～ 2026-10-02（1 天）") ||
		!strings.Contains(out, "孵蛋時的 commit 組成：docs 2（50%）、feat 1（25%）、fix 1（25%），共 4 筆") || strings.Contains(out, "地下城") {
		t.Fatalf("%v\n%s", err, out)
	}
	// --author 過濾後沒有 commit
	if out, err := dnd(t, "sim", repo, "--author", "nobody@example.invalid"); err != nil || !strings.Contains(out, "沒有可以重播的 commit") {
		t.Fatalf("%v %q", err, out)
	}
	for _, args := range [][]string{
		{"sim", t.TempDir()}, {"sim", repo, "--species", "unicorn"}, {"sim", repo, "--from", "10/02"}, {"sim", repo, "--ai-turns-per-commit", "-1"}, {"sim", "a", "b"},
	} {
		if _, err := dnd(t, args...); err == nil {
			t.Fatalf("%v 應報錯", args)
		}
	}
}
