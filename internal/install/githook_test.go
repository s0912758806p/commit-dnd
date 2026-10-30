package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const dnd = "/opt/it's/dnd"

func hookPath(dir string) string { return filepath.Join(dir, hookName) }

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallCreatesAndRemoves(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hooks") // 目錄不存在時要建立（core.hooksPath 常見）
	if _, err := InstallGitHook(dir, dnd); err != nil {
		t.Fatal(err)
	}
	got := read(t, hookPath(dir))
	if got != shebang+"\n"+Block(dnd) {
		t.Fatalf("內容 = %q", got)
	}
	if fi, _ := os.Stat(hookPath(dir)); fi.Mode()&0o111 == 0 {
		t.Fatal("hook 沒有執行權限")
	}
	if !strings.Contains(got, `'/opt/it'\''s/dnd' hook git`) {
		t.Fatalf("路徑沒有正確跳脫: %q", got)
	}

	if msg, _ := InstallGitHook(dir, dnd); !strings.HasPrefix(msg, "已經安裝過") {
		t.Fatalf("重複安裝應該不變: %q", msg)
	}
	if _, err := UninstallGitHook(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookPath(dir)); !os.IsNotExist(err) {
		t.Fatal("只剩 shebang 時應刪除檔案")
	}
	if msg, err := UninstallGitHook(dir); err != nil || !strings.HasPrefix(msg, "沒有安裝") {
		t.Fatalf("msg=%q err=%v", msg, err)
	}
}

// 既有 hook：區塊插在 shebang 後（原腳本中途 exit 也不影響），移除後原樣還原。
func TestInstallKeepsExistingHook(t *testing.T) {
	dir := t.TempDir()
	orig := "#!/usr/bin/env bash\nset -e\nnpm run lint\nexit 0\n"
	if err := os.WriteFile(hookPath(dir), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallGitHook(dir, dnd); err != nil {
		t.Fatal(err)
	}
	want := "#!/usr/bin/env bash\n" + Block(dnd) + "set -e\nnpm run lint\nexit 0\n"
	if got := read(t, hookPath(dir)); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if fi, _ := os.Stat(hookPath(dir)); fi.Mode()&0o111 == 0 {
		t.Fatal("應補上執行權限")
	}

	if _, err := InstallGitHook(dir, "/new/dnd"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, hookPath(dir)); !strings.Contains(got, "'/new/dnd'") || strings.Contains(got, dnd) {
		t.Fatalf("應更新路徑: %q", got)
	}

	if _, err := UninstallGitHook(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, hookPath(dir)); got != orig {
		t.Fatalf("移除後應還原: %q", got)
	}
}

func TestInstallRefusesNonShellHook(t *testing.T) {
	dir := t.TempDir()
	orig := "#!/usr/bin/env node\nconsole.log(1)\n"
	os.WriteFile(hookPath(dir), []byte(orig), 0o755)
	if _, err := InstallGitHook(dir, dnd); err == nil {
		t.Fatal("非 shell 腳本應拒絕")
	}
	if read(t, hookPath(dir)) != orig {
		t.Fatal("拒絕時不可改檔")
	}
}

func TestIsShell(t *testing.T) {
	for line, want := range map[string]bool{
		"#!/bin/sh": true, "#!/bin/bash -e": true, "#! /bin/zsh": true, "#!/usr/bin/env bash": true,
		"#!/usr/bin/env node": false, "#!/usr/bin/python3": false, "echo hi": false, "#!": false,
	} {
		if isShell(line) != want {
			t.Errorf("isShell(%q) != %v", line, want)
		}
	}
}

func TestGitHooksDir(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	dir, err := GitHooksDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(filepath.Join(repo, ".git", "hooks")); mustEval(t, dir) != want {
		t.Fatalf("dir = %q", dir)
	}

	git("config", "core.hooksPath", ".husky")
	dir, _ = GitHooksDir(repo)
	if want, _ := filepath.EvalSymlinks(repo); filepath.Join(want, ".husky") != filepath.Join(mustEval(t, filepath.Dir(dir)), ".husky") {
		t.Fatalf("應尊重 core.hooksPath: %q", dir)
	}

	if _, err := GitHooksDir(t.TempDir()); err == nil {
		t.Fatal("非 repo 應回傳錯誤")
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
