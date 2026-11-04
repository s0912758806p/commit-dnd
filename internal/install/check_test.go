package install

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitHookStatus(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := GitHookStatus(dir); ok || err != nil {
		t.Fatalf("沒有 hook 時: ok=%v err=%v", ok, err)
	}
	if _, err := InstallGitHook(dir, "/opt/it's/dnd"); err != nil {
		t.Fatal(err)
	}
	p, ok, err := GitHookStatus(dir)
	if !ok || err != nil || p != "/opt/it's/dnd" {
		t.Fatalf("p=%q ok=%v err=%v", p, ok, err)
	}
}

func TestClaudeSettingsStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if st, err := ClaudeSettingsStatus(path); err != nil || st.PostToolUse || st.Statusline {
		t.Fatalf("檔案不存在: %+v %v", st, err)
	}
	settings := `{
  "hooks": {
    "PreToolUse": [{"hooks": [{"type": "command", "command": "~/.claude/hooks/gate"}]}],
    "PostToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "'/u/go/bin/dnd' hook claude", "async": true}]}]
  },
  "statusLine": {"type": "command", "command": "'/u/go/bin/dnd' statusline"}
}`
	os.WriteFile(path, []byte(settings), 0o600)
	st, err := ClaudeSettingsStatus(path)
	if err != nil || !st.PostToolUse || st.Stop || !st.Statusline || !slices.Equal(st.DndPaths, []string{"/u/go/bin/dnd"}) {
		t.Fatalf("%+v %v", st, err)
	}
	os.WriteFile(path, []byte("{"), 0o600)
	if _, err := ClaudeSettingsStatus(path); err == nil {
		t.Fatal("壞掉的 JSON 應回傳錯誤")
	}
}

func TestUnquoteFirstWord(t *testing.T) {
	for in, want := range map[string]string{
		`'/opt/it'\''s/dnd' hook claude`: "/opt/it's/dnd",
		`"/x y/dnd" statusline`:          "/x y/dnd",
		`dnd statusline`:                 "dnd",
		``:                               "",
	} {
		if got := unquoteFirstWord(in); got != want {
			t.Errorf("unquoteFirstWord(%q) = %q, want %q", in, got, want)
		}
	}
}
