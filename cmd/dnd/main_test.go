package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// dnd 執行 CLI 並回傳輸出；stdin 為空（非互動）。
func dnd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func commit(t *testing.T, repo, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(msg+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", msg}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestHatchCommitStatusLog(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	repo := gitRepo(t)
	t.Chdir(repo)

	if _, err := dnd(t, "hatch"); err == nil || !strings.Contains(err.Error(), "--species") {
		t.Fatalf("非互動且沒給物種應報錯: %v", err)
	}
	if _, err := dnd(t, "hatch", "--species", "unicorn"); err == nil {
		t.Fatal("未知物種應報錯")
	}
	if out, err := dnd(t, "hatch", "--species", "elf", "--name", "小橘"); err != nil || !strings.Contains(out, "小橘") {
		t.Fatalf("hatch: %v\n%s", err, out)
	}
	if _, err := dnd(t, "hatch", "--species", "orc"); err == nil {
		t.Fatal("已有寵物應報錯")
	}

	commit(t, repo, "docs: first") // 最低的類型也要升到 Lv 2
	if out, err := dnd(t, "hook", "git"); err != nil || out != "" {
		t.Fatalf("hook 應安靜成功: %v %q", err, out)
	}
	out, err := dnd(t, "status")
	if err != nil || !strings.Contains(out, "名字  小橘") || !strings.Contains(out, "等級  Lv 2") {
		t.Fatalf("status: %v\n%s", err, out)
	}
	out, err = dnd(t, "log")
	if err != nil || !strings.Contains(out, "誕生了") || !strings.Contains(out, "升到了 Lv 2") {
		t.Fatalf("log: %v\n%s", err, out)
	}
}

// 任何輸入都不能讓 hook 失敗：不是 repo 時 exit 0、沒有輸出，錯誤寫進 errors.log。
func TestHookNeverFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	t.Chdir(t.TempDir())
	if out, err := dnd(t, "hook", "git", "unexpected", "args"); err != nil || out != "" {
		t.Fatalf("err=%v out=%q", err, out)
	}
	b, err := os.ReadFile(filepath.Join(home, "errors.log"))
	if err != nil || !strings.Contains(string(b), "[hook git]") {
		t.Fatalf("errors.log: %v %q", err, b)
	}
}

func TestInstallUninstallGit(t *testing.T) {
	repo := gitRepo(t)
	// 測試執行檔在 go-build 暫存目錄，install 應拒絕；這裡只驗證 uninstall 與錯誤訊息。
	if _, err := dnd(t, "install", "git", "--repo", repo); err == nil || !strings.Contains(err.Error(), "make install") {
		t.Fatalf("go test／go run 的執行檔不應安裝: %v", err)
	}
	if out, err := dnd(t, "uninstall", "git", "--repo", repo); err != nil || !strings.Contains(out, "沒有安裝") {
		t.Fatalf("uninstall: %v %q", err, out)
	}
	if _, err := dnd(t, "uninstall", "git", "--repo", t.TempDir()); err == nil {
		t.Fatal("非 repo 應報錯")
	}
}

func TestDoctor(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	repo := gitRepo(t)
	settings := filepath.Join(t.TempDir(), "settings.json")

	out, err := dnd(t, "doctor", "--repo", repo, "--settings", settings)
	if err == nil || !strings.Contains(out, "✗ 寵物") || !strings.Contains(out, "✗ git hook") || !strings.Contains(out, "✗ Claude Code") {
		t.Fatalf("全新環境應列出問題: %v\n%s", err, out)
	}

	if _, err := dnd(t, "hatch", "--species", "dwarf"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(settings, []byte(`{"statusLine":{"type":"command","command":"'/nowhere/dnd' statusline"}}`), 0o600)
	out, _ = dnd(t, "doctor", "--repo", repo, "--settings", settings)
	if !strings.Contains(out, "✓ 寵物") || !strings.Contains(out, "/nowhere/dnd 不存在") || !strings.Contains(out, "PostToolUse hook、Stop hook 未安裝") {
		t.Fatalf("應指出缺少的 hook 與失效路徑:\n%s", out)
	}
}

// statusline 在沒有寵物、資料壞掉時也要輸出一行並成功（Claude Code 只顯示 exit 0 的輸出）。
func TestStatuslineNeverFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	out, err := dnd(t, "statusline")
	if err != nil || out != "dnd：還沒有寵物（dnd hatch）" {
		t.Fatalf("err=%v out=%q", err, out)
	}
	os.WriteFile(filepath.Join(home, "state.json"), []byte("{"), 0o600)
	if out, err := dnd(t, "statusline"); err != nil || !strings.Contains(out, "讀取失敗") {
		t.Fatalf("err=%v out=%q", err, out)
	}
}

func TestStatuslineChainsOriginal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	if _, err := dnd(t, "hatch", "--species", "elf", "--name", "小精"); err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(home, "claude", "original-statusline.json")
	os.MkdirAll(filepath.Dir(orig), 0o755)
	// 原本的 statusLine 會收到同一份 stdin
	os.WriteFile(orig, []byte(`{"type":"command","command":"printf 'orig:'; cat"}`), 0o600)
	root := newRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetIn(strings.NewReader(`{"model":"x"}`))
	root.SetArgs([]string{"statusline"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.HasPrefix(got, `orig:{"model":"x"}  小精 Lv 1`) {
		t.Fatalf("got %q", got)
	}
}

// Claude Code 的 Stop hook 若 exit 2 會讓 Claude 停不下來：hook claude 對任何輸入都必須安靜成功。
func TestHookClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	for _, in := range []string{
		`{"hook_event_name":"Stop","cwd":"/x/commit-dnd"}`,
		`{"hook_event_name":"PostToolUse","tool_name":"Edit","cwd":"/x/commit-dnd"}`,
		`not json`, ``,
	} {
		root := newRoot()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetIn(strings.NewReader(in))
		root.SetArgs([]string{"hook", "claude"})
		if err := root.Execute(); err != nil || buf.Len() != 0 {
			t.Fatalf("%q: err=%v out=%q", in, err, buf.String())
		}
	}
	out, _ := dnd(t, "statusline")
	if !strings.Contains(out, "還沒有寵物") {
		t.Fatalf("%q", out)
	}
	b, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if n := bytes.Count(b, []byte("\n")); n != 2 {
		t.Fatalf("應寫入 2 個事件，實際 %d:\n%s", n, b)
	}
}

func TestDungeon(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	if out, err := dnd(t, "dungeon"); err != nil || out != "還沒有寵物。用 dnd hatch 孵一隻吧。\n" {
		t.Fatalf("沒有寵物時應提示孵蛋: %v %q", err, out)
	}
	if _, err := dnd(t, "hatch", "--species", "elf", "--name", "小精"); err != nil {
		t.Fatal(err)
	}
	if out, err := dnd(t, "dungeon"); err != nil || !strings.HasPrefix(out, "樓層  入口\n") {
		t.Fatalf("剛孵出來應在入口: %v\n%s", err, out)
	}
}

// warp 的參數由 CLI 檢查；能不能傳送由引擎判斷，這裡不測。
func TestWarpArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	for _, args := range [][]string{{"warp"}, {"warp", "abc"}, {"warp", "-3"}, {"warp", "1.5"}, {"warp", "10", "20"}} {
		if _, err := dnd(t, args...); err == nil {
			t.Fatalf("%v 應報錯", args)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("參數錯誤時不應寫入事件: %v", err)
	}
}
