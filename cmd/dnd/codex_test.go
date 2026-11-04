package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/install"
)

const codexPayload = `{"type":"agent-turn-complete","thread-id":"t","turn-id":"1","cwd":"/x/commit-dnd","input-messages":[],"last-assistant-message":null}`

// runHookCodex 執行 `dnd hook codex args...`，hook 不可輸出也不可失敗。
func runHookCodex(t *testing.T, args ...string) {
	t.Helper()
	root := newRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"hook", "codex"}, args...))
	if err := root.Execute(); err != nil || buf.Len() != 0 {
		t.Fatalf("%q: err=%v out=%q", args, err, buf.String())
	}
}

func TestHookCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	runHookCodex(t, codexPayload)
	runHookCodex(t, `{"type":"something-else","cwd":"/x"}`)
	runHookCodex(t, "not json")
	runHookCodex(t, "--help")
	runHookCodex(t)

	b, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"source":"codex","type":"tool_use","repo":"commit-dnd","tool":"Codex"`) ||
		!strings.Contains(lines[1], `"source":"codex","type":"turn","repo":"commit-dnd"`) {
		t.Fatalf("一個 Codex 回合應寫入攻擊＋回合兩個事件:\n%s", b)
	}
	if strings.Contains(string(b), "input-messages") {
		t.Fatal("不可保存對話內容")
	}
	log, _ := os.ReadFile(filepath.Join(home, "errors.log"))
	if !strings.Contains(string(log), "[hook codex] invalid character") || !strings.Contains(string(log), "沒有收到 notify payload") {
		t.Fatalf("錯誤應寫進 errors.log:\n%s", log)
	}

	// capture 目錄存在時保存原始 payload
	os.MkdirAll(filepath.Join(home, "capture", "codex"), 0o755)
	runHookCodex(t, codexPayload)
	if got, _ := os.ReadFile(filepath.Join(home, "capture", "codex", "agent-turn-complete.json")); string(got) != codexPayload {
		t.Fatalf("capture: %q", got)
	}
}

// 有串接時，dnd 處理完自己的事件後用同樣的參數執行原本的 notify；dnd 失敗也照樣執行。
func TestHookCodexChainsOriginal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	out := filepath.Join(t.TempDir(), "args")
	orig := `{"notify":["/bin/sh","-c","printf '%s\\n' \"$@\" > '` + out + `'","sh","extra"]}`
	os.MkdirAll(filepath.Join(home, "codex"), 0o755)
	if err := os.WriteFile(install.OriginalNotifyPath(home), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{codexPayload, "not json"} {
		os.Remove(out)
		runHookCodex(t, payload)
		var got []byte
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if got, _ = os.ReadFile(out); bytes.HasSuffix(got, []byte("\n")) {
				break
			}
		}
		if string(got) != "extra\n"+payload+"\n" {
			t.Fatalf("原本的 notify 應收到同樣的參數: %q", got)
		}
	}

	// 原本的 notify 不存在：錯誤寫進 errors.log，dnd 的事件照寫
	os.WriteFile(install.OriginalNotifyPath(home), []byte(`{"notify":["/nowhere/notify"]}`), 0o644)
	runHookCodex(t, codexPayload)
	log, _ := os.ReadFile(filepath.Join(home, "errors.log"))
	if !strings.Contains(string(log), "[hook codex notify 串接]") {
		t.Fatalf("串接失敗應寫進 errors.log:\n%s", log)
	}
	b, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if n := bytes.Count(b, []byte("\n")); n != 4 {
		t.Fatalf("應有 4 個事件，實際 %d", n)
	}
}

// install codex 需要真正安裝好的 dnd 路徑，細節在 internal/install 測；這裡測 CLI 的設定檔位置與確認流程。
func TestUninstallCodexCLI(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	cfg := t.TempDir()
	t.Setenv("CODEX_HOME", cfg)
	path := filepath.Join(cfg, "config.toml")
	orig := "model = \"gpt-5-codex\"\nnotify = [\"/x/dnd\", \"hook\", \"codex\"]\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := dnd(t, "uninstall", "codex")
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(out, `-notify = ["/x/dnd", "hook", "codex"]`) {
		t.Fatalf("非互動沒給 --yes 應拒絕並先印 diff: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(path); string(b) != orig {
		t.Fatal("拒絕時不可改檔")
	}
	if out, err := dnd(t, "uninstall", "codex", "--yes"); err != nil || !strings.Contains(out, "已從 "+path+" 移除 dnd") {
		t.Fatalf("%v\n%s", err, out)
	}
	if b, _ := os.ReadFile(path); string(b) != "model = \"gpt-5-codex\"\n" {
		t.Fatalf("got %q", b)
	}
	if out, err := dnd(t, "uninstall", "codex", "--config", path); err != nil || !strings.Contains(out, "沒有安裝") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := dnd(t, "install", "codex", "--config", path); err == nil || !strings.Contains(err.Error(), "go run") {
		t.Fatalf("測試執行檔不能拿來當 notify: %v", err)
	}
}

func TestDoctorCodex(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	cfg := filepath.Join(t.TempDir(), "config.toml")
	settings := filepath.Join(t.TempDir(), "settings.json")
	doctor := func() string {
		out, _ := dnd(t, "doctor", "--repo", t.TempDir(), "--settings", settings, "--codex-config", cfg)
		return out
	}
	if out := doctor(); !strings.Contains(out, "! Codex：沒有 "+cfg) || strings.Contains(out, "✗ Codex") {
		t.Fatalf("沒用 Codex 時只提醒:\n%s", out)
	}
	os.WriteFile(cfg, []byte("model = \"a\"\n"), 0o600)
	if out := doctor(); !strings.Contains(out, "! Codex：notify 沒有接上 dnd") {
		t.Fatalf("%s", out)
	}
	os.WriteFile(cfg, []byte("notify = [\"/nowhere/dnd\", \"hook\", \"codex\"]\n"), 0o600)
	if out := doctor(); !strings.Contains(out, "✗ Codex：notify 指向的 /nowhere/dnd 不存在") {
		t.Fatalf("%s", out)
	}
	os.WriteFile(cfg, []byte("notify = "), 0o600)
	if out := doctor(); !strings.Contains(out, "✗ Codex：讀不了") {
		t.Fatalf("%s", out)
	}
}

// 用正式的 content：Codex 第一個回合遇到怪物，第二個回合寵物先出手（"Codex" 在 attack_tools）。
func TestCodexTurnAttacks(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	if _, err := dnd(t, "hatch", "--species", "orc"); err != nil {
		t.Fatal(err)
	}
	load := func() (st struct{ Kills, HP, MaxHP int }) {
		s, _, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		state, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		d := state.Dungeon
		return struct{ Kills, HP, MaxHP int }{d.Kills, d.Monster.HP, d.Monster.MaxHP}
	}
	runHookCodex(t, codexPayload)
	if st := load(); st.MaxHP == 0 || st.HP != st.MaxHP {
		t.Fatalf("第一個回合應遇到怪物、還沒攻擊: %+v", st)
	}
	runHookCodex(t, codexPayload)
	if st := load(); st.Kills == 0 && st.HP >= st.MaxHP {
		t.Fatalf("第二個回合應攻擊: %+v", st)
	}
}
