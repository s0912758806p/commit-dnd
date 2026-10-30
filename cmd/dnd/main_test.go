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
	if out, err := dnd(t, "hatch", "--species", "slime", "--name", "小橘"); err != nil || !strings.Contains(out, "小橘") {
		t.Fatalf("hatch: %v\n%s", err, out)
	}
	if _, err := dnd(t, "hatch", "--species", "owl"); err == nil {
		t.Fatal("已有寵物應報錯")
	}

	commit(t, repo, "docs: first") // 最低的類型也要升到 Lv 2
	if out, err := dnd(t, "hook", "git"); err != nil || out != "" {
		t.Fatalf("hook 應安靜成功: %v %q", err, out)
	}
	out, err := dnd(t, "status")
	if err != nil || !strings.Contains(out, "小橘（史萊姆）Lv 2") {
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
