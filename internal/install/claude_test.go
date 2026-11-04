package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// userSettings 與使用者真實 settings.json 同形狀：2 格縮排、key 不照字母排序、
// 既有 PreToolUse／SessionStart hooks、別人的 statusLine（帶 padding）、env、permissions。
const userSettings = `{
  "env": {
    "FOO": "a<b>&c",
    "N": 1.50
  },
  "autoMode": true,
  "effortLevel": "high",
  "enabledPlugins": {
    "superpowers@market": true
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/guard.sh"
          }
        ]
      },
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/fmt.sh",
            "timeout": 30
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo \"hi\" \u00e9"
          }
        ]
      }
    ]
  },
  "model": "opus",
  "permissions": {
    "allow": ["Bash(git status)", "Read"],
    "deny": []
  },
  "statusLine": {
    "type": "command",
    "command": "bash /Users/me/.claude/statusline-command.sh",
    "padding": 0
  },
  "theme": "dark"
}
`

const dndBin = "/Users/me/go/bin/dnd"

func setup(t *testing.T, content string, mode os.FileMode) (path, dndHome string) {
	t.Helper()
	dir := t.TempDir()
	dndHome = filepath.Join(dir, "dndhome")
	t.Setenv("DND_HOME", dndHome)
	path = filepath.Join(dir, "claude", "settings.json")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	return path, dndHome
}

var yes = Confirm{Out: &bytes.Buffer{}, Yes: true, Now: time.Date(2026, 11, 4, 9, 8, 7, 0, time.UTC)}

func apply(t *testing.T, p *ClaudePlan) string {
	t.Helper()
	var out bytes.Buffer
	c := yes
	c.Out = &out
	if ok, err := p.Apply(c); err != nil || !ok {
		t.Fatalf("apply: ok=%v err=%v\n%s", ok, err, out.String())
	}
	return out.String()
}

func install(t *testing.T, path, dndPath, dndHome string) string {
	t.Helper()
	p, err := PlanClaudeInstall(path, dndPath, dndHome)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Changed() {
		t.Fatal("應該有修改")
	}
	return apply(t, p)
}

func uninstall(t *testing.T, path, dndHome string) string {
	t.Helper()
	p, err := PlanClaudeUninstall(path, dndHome)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Changed() {
		t.Fatal("應該有修改")
	}
	return apply(t, p)
}

// topKeys 依出現順序回傳最外層的 key。
func topKeys(t *testing.T, b []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	var keys []string
	dec.Token()
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestClaudeInstallUserShape(t *testing.T) {
	path, dndHome := setup(t, userSettings, 0o600)
	out := install(t, path, dndBin, dndHome)
	if !strings.Contains(out, "+++ "+path) || !strings.Contains(out, "已備份到 ") {
		t.Fatalf("應印出 diff 與備份路徑:\n%s", out)
	}
	got := read(t, path)

	// 其他 key 的順序、內容、縮排不變；hooks 與 statusLine 在原位置
	if k := strings.Join(topKeys(t, []byte(got)), ","); k != "env,autoMode,effortLevel,enabledPlugins,hooks,model,permissions,statusLine,theme" {
		t.Fatalf("key 順序被打亂: %s", k)
	}
	for _, keep := range []string{
		"  \"env\": {\n    \"FOO\": \"a<b>&c\",\n    \"N\": 1.50\n  },\n",
		`"allow": ["Bash(git status)", "Read"],`,
		`"command": "echo \"hi\" \u00e9"`,
		"    \"PreToolUse\": [\n      {\n        \"matcher\": \"Bash\",",
	} {
		if !strings.Contains(got, keep) {
			t.Fatalf("原內容被改動，缺少 %q:\n%s", keep, got)
		}
	}

	hook := HookCommand(dndBin)
	var s struct {
		Hooks map[string][]struct {
			Matcher *string
			Hooks   []map[string]any
		}
		StatusLine map[string]any
	}
	if err := json.Unmarshal([]byte(got), &s); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"PostToolUse", "Stop"} {
		g := s.Hooks[ev]
		if len(g) != 1 || len(g[0].Hooks) != 1 || g[0].Hooks[0]["command"] != hook ||
			g[0].Hooks[0]["async"] != true || g[0].Hooks[0]["timeout"] != 10.0 {
			t.Fatalf("%s = %+v", ev, g)
		}
		if (ev == "PostToolUse") != (g[0].Matcher != nil && *g[0].Matcher == "*") {
			t.Fatalf("%s matcher = %v", ev, g[0].Matcher)
		}
	}
	if len(s.Hooks["PreToolUse"]) != 2 || len(s.Hooks["SessionStart"]) != 1 {
		t.Fatal("既有 hooks 不可變")
	}
	if s.StatusLine["command"] != StatuslineCommand(dndBin) || s.StatusLine["padding"] != 0.0 {
		t.Fatalf("statusLine = %v", s.StatusLine)
	}
	// 新加的區塊照檔案縮排排版
	if !strings.Contains(got, "\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",") {
		t.Fatalf("新區塊縮排不對:\n%s", got)
	}

	// 原 statusLine 原樣保存
	var orig map[string]any
	if err := json.Unmarshal([]byte(read(t, OriginalStatuslinePath(dndHome))), &orig); err != nil {
		t.Fatal(err)
	}
	if orig["command"] != "bash /Users/me/.claude/statusline-command.sh" || orig["padding"] != 0.0 || len(orig) != 3 {
		t.Fatalf("原 statusLine = %v", orig)
	}

	// 權限保留
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("權限 = %v", fi.Mode().Perm())
	}

	// 重複安裝：沒有修改
	if p, err := PlanClaudeInstall(path, dndBin, dndHome); err != nil || p.Changed() {
		t.Fatalf("重複安裝應該不變: err=%v\n%s", err, p.Diff())
	}

	// 移除後逐位元組還原，原 statusLine 檔被刪除
	uninstall(t, path, dndHome)
	if got := read(t, path); got != userSettings {
		t.Fatalf("移除後應與安裝前相同:\n%s", unifiedDiff("settings", []byte(userSettings), []byte(got)))
	}
	if _, err := os.Stat(OriginalStatuslinePath(dndHome)); !os.IsNotExist(err) {
		t.Fatal("還原後應刪除原 statusLine 檔")
	}
	if p, _ := PlanClaudeUninstall(path, dndHome); p.Changed() {
		t.Fatal("重複移除應該不變")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("權限 = %v", fi.Mode().Perm())
	}
}

func TestClaudeInstallUpdatesDndPath(t *testing.T) {
	path, dndHome := setup(t, userSettings, 0o644)
	install(t, path, dndBin, dndHome)
	install(t, path, "/opt/new/dnd", dndHome)
	got := read(t, path)
	if strings.Contains(got, dndBin) || strings.Count(got, HookCommand("/opt/new/dnd")) != 2 ||
		!strings.Contains(got, StatuslineCommand("/opt/new/dnd")) {
		t.Fatalf("應更新路徑且不重複:\n%s", got)
	}
	// 換路徑時原 statusLine 檔不能被 dnd 自己蓋掉
	if !strings.Contains(read(t, OriginalStatuslinePath(dndHome)), "statusline-command.sh") {
		t.Fatal("原 statusLine 檔被覆蓋")
	}
	uninstall(t, path, dndHome)
	if read(t, path) != userSettings {
		t.Fatal("移除後應還原")
	}
}

// 使用者自己把 dnd hook 搬進別的 group、或在同一 group 放了別的 hook：只移除 dnd 的那一個。
func TestClaudeUninstallKeepsUserGroups(t *testing.T) {
	content := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "say done"
          },
          {
            "type": "command",
            "command": "'/x/dnd' hook claude"
          }
        ]
      },
      {
        "hooks": []
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "/x/dnd statusline"
  }
}
`
	path, dndHome := setup(t, content, 0o600)
	p, err := PlanClaudeInstall(path, "/x/dnd", dndHome)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Changed() || strings.Count(string(p.New), "hook claude") != 2 {
		t.Fatalf("Stop 已有 dnd，只該補 PostToolUse:\n%s", p.Diff())
	}

	uninstall(t, path, dndHome)
	want := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "say done"
          }
        ]
      },
      {
        "hooks": []
      }
    ]
  }
}
`
	if got := read(t, path); got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestClaudeInstallNewFile(t *testing.T) {
	path, dndHome := setup(t, "", 0)
	out := install(t, path, dndBin, dndHome)
	if strings.Contains(out, "已備份") {
		t.Fatal("新檔不需要備份")
	}
	got := read(t, path)
	if k := strings.Join(topKeys(t, []byte(got)), ","); k != "hooks,statusLine" {
		t.Fatalf("keys = %s\n%s", k, got)
	}
	if !strings.HasPrefix(got, "{\n  \"hooks\": {\n    \"PostToolUse\": [\n      {\n        \"matcher\": \"*\",") || !strings.HasSuffix(got, "}\n") {
		t.Fatalf("排版不對:\n%s", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("新檔權限 = %v", fi.Mode().Perm())
	}
	if _, err := os.Stat(OriginalStatuslinePath(dndHome)); !os.IsNotExist(err) {
		t.Fatal("沒有原 statusLine 時不該建檔")
	}

	// 沒有原 statusLine：uninstall 刪掉 key，hooks 也被清空而刪掉
	uninstall(t, path, dndHome)
	if got := read(t, path); got != "{}\n" {
		t.Fatalf("got %q", got)
	}
}

func TestClaudeRejectsInvalidJSON(t *testing.T) {
	for _, bad := range []string{`{"hooks": {,}`, `[]`, `{"hooks": []}`, `{"hooks": {"Stop": {}}}`} {
		path, dndHome := setup(t, bad, 0o600)
		if _, err := PlanClaudeInstall(path, dndBin, dndHome); err == nil {
			t.Errorf("%s: 應該報錯", bad)
		}
		if read(t, path) != bad {
			t.Errorf("%s: 不可改檔", bad)
		}
	}
	path, dndHome := setup(t, `{"a":`, 0o600)
	if _, err := PlanClaudeUninstall(path, dndHome); err == nil {
		t.Error("uninstall 也要拒絕壞 JSON")
	}
}

func TestClaudeSettingsPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if p, _ := ClaudeSettingsPath(""); p != filepath.Join(dir, "settings.json") {
		t.Fatalf("CLAUDE_CONFIG_DIR: %s", p)
	}
	if p, _ := ClaudeSettingsPath("/x/s.json"); p != "/x/s.json" {
		t.Fatalf("--settings 應優先: %s", p)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", dir)
	if p, _ := ClaudeSettingsPath(""); p != filepath.Join(dir, ".claude", "settings.json") {
		t.Fatalf("預設: %s", p)
	}
}

func TestClaudeConfirm(t *testing.T) {
	path, dndHome := setup(t, userSettings, 0o600)
	p, err := PlanClaudeInstall(path, dndBin, dndHome)
	if err != nil {
		t.Fatal(err)
	}

	// 非互動又沒 --yes：拒絕，不寫任何檔
	var out bytes.Buffer
	if ok, err := p.Apply(Confirm{Out: &out, In: strings.NewReader("y\n")}); ok || err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	// 互動但回答 N
	if ok, err := p.Apply(Confirm{Out: &out, In: strings.NewReader("\n"), Interactive: true}); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out.String(), "要寫入 "+path+" 嗎？[y/N]") {
		t.Fatalf("應詢問:\n%s", out.String())
	}
	assertUntouched(t, path, dndHome)

	// 確認期間檔案被改：放棄
	os.WriteFile(path, []byte(userSettings+" "), 0o600)
	if ok, err := p.Apply(Confirm{Out: &out, In: strings.NewReader("y\n"), Interactive: true}); ok || err == nil {
		t.Fatalf("檔案被改應放棄: ok=%v err=%v", ok, err)
	}
	os.WriteFile(path, []byte(userSettings), 0o600)

	// 互動回答 y：備份與原檔同內容、同權限
	out.Reset()
	if ok, err := p.Apply(Confirm{Out: &out, In: strings.NewReader("y\n"), Interactive: true, Now: yes.Now}); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	backup := path + ".dnd-backup-20261104-090807"
	if !strings.Contains(out.String(), "已備份到 "+backup) {
		t.Fatalf("應印出備份路徑:\n%s", out.String())
	}
	if read(t, backup) != userSettings {
		t.Fatal("備份內容應等於原檔")
	}
	if fi, _ := os.Stat(backup); fi.Mode().Perm() != 0o600 {
		t.Fatalf("備份權限 = %v", fi.Mode().Perm())
	}

	// 同一秒再寫一次不覆蓋舊備份
	out.Reset()
	p, _ = PlanClaudeUninstall(path, dndHome)
	p.Apply(Confirm{Out: &out, Yes: true, Now: yes.Now})
	if !strings.Contains(out.String(), backup+"-2") || read(t, backup) != userSettings {
		t.Fatalf("同一秒的備份應加序號:\n%s", out.String())
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings.json.dnd-*")); len(m) != 0 {
		t.Fatalf("暫存檔殘留: %v", m)
	}
}

func assertUntouched(t *testing.T, path, dndHome string) {
	t.Helper()
	if read(t, path) != userSettings {
		t.Fatal("不可改檔")
	}
	if m, _ := filepath.Glob(path + ".dnd-backup-*"); len(m) != 0 {
		t.Fatal("不該有備份")
	}
	if _, err := os.Stat(OriginalStatuslinePath(dndHome)); !os.IsNotExist(err) {
		t.Fatal("不該存原 statusLine")
	}
}

// settings.json 是 symlink（dotfiles 管理）時寫到目標檔，symlink 保留。
func TestClaudeWritesThroughSymlink(t *testing.T) {
	real, dndHome := setup(t, userSettings, 0o600)
	link := filepath.Join(t.TempDir(), "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	install(t, link, dndBin, dndHome)
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink 被換成一般檔")
	}
	if !strings.Contains(read(t, real), "hook claude") {
		t.Fatal("應寫入 symlink 指向的檔案")
	}
}

func TestClaudeKeepsCompactAndTabs(t *testing.T) {
	path, dndHome := setup(t, `{"model":"opus","statusLine":{"type":"command","command":"x"}}`, 0o600)
	p, _ := PlanClaudeInstall(path, dndBin, dndHome)
	if bytes.Contains(p.New, []byte("\n")) || !bytes.HasPrefix(p.New, []byte(`{"model":"opus","statusLine":{"type":"command","command":`)) {
		t.Fatalf("單行檔應維持單行: %s", p.New)
	}

	tabbed := "{\n\t\"model\": \"opus\"\n}\n"
	path, dndHome = setup(t, tabbed, 0o600)
	p, _ = PlanClaudeInstall(path, dndBin, dndHome)
	if !bytes.Contains(p.New, []byte("\n\t\"hooks\": {\n\t\t\"PostToolUse\": [")) {
		t.Fatalf("應沿用 tab 縮排:\n%s", p.New)
	}
	apply(t, p)
	uninstall(t, path, dndHome)
	if read(t, path) != tabbed {
		t.Fatalf("got %q", read(t, path))
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16\n")
	b := []byte("1\n2\nX\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n16\n")
	want := "--- f\n+++ f\n@@ -1,5 +1,6 @@\n 1\n 2\n+X\n 3\n 4\n 5\n@@ -12,5 +13,4 @@\n 12\n 13\n 14\n-15\n 16\n"
	if got := unifiedDiff("f", a, b); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := unifiedDiff("f", nil, []byte("{}\n")); got != "--- f\n+++ f\n@@ -0,0 +1,1 @@\n+{}\n" {
		t.Fatalf("新檔 diff = %q", got)
	}
	if unifiedDiff("f", a, a) != "" {
		t.Fatal("相同內容不該有 diff")
	}
	if !regexp.MustCompile(`(?m)^@@ -4,7 \+4,7 @@$`).MatchString(unifiedDiff("f", a, bytes.Replace(a, []byte("7\n"), []byte("seven\n"), 1))) {
		t.Fatal("hunk 範圍不對")
	}
}
