package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// install claude 需要真正安裝好的 dnd 路徑（測試執行檔在 go-build 底下會被 selfPath 拒絕），
// 細節在 internal/install 測；這裡測 CLI 的設定檔位置與確認流程。
func TestUninstallClaudeCLI(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	path := filepath.Join(cfg, "settings.json")
	orig := "{\n  \"model\": \"opus\",\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"'/x/dnd' statusline\"\n  }\n}\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := dnd(t, "uninstall", "claude")
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(out, "-  \"statusLine\": {") {
		t.Fatalf("非互動沒給 --yes 應拒絕並先印 diff: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(path); string(b) != orig {
		t.Fatal("拒絕時不可改檔")
	}

	if out, err := dnd(t, "uninstall", "claude", "--yes"); err != nil || !strings.Contains(out, "已從 "+path+" 移除 dnd") {
		t.Fatalf("%v\n%s", err, out)
	}
	if b, _ := os.ReadFile(path); string(b) != "{\n  \"model\": \"opus\"\n}\n" {
		t.Fatalf("got %q", b)
	}
	if out, err := dnd(t, "uninstall", "claude", "--settings", path); err != nil || !strings.Contains(out, "沒有安裝") {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := dnd(t, "install", "claude", "--settings", path); err == nil || !strings.Contains(err.Error(), "go run") {
		t.Fatalf("測試執行檔不能拿來當 hook: %v", err)
	}
}
