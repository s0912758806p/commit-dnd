package install

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const testDnd = "/u/go/bin/dnd"

// 使用者形狀的 config.toml（依官方文件的欄位手寫）：註解、最上層 key、table、多行陣列。
const userConfig = `# Codex 設定
model = "gpt-5-codex"   # 預設模型
approval_policy = "on-request"
sandbox_mode = 'workspace-write'

[mcp_servers.docs]
command = "npx"
args = [
  "-y", # 套件
  "docs-mcp",
]

[profiles.fast]
model = "gpt-5-mini"
`

func codexFixture(t *testing.T, content string) (path, dndHome string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "config.toml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return path, filepath.Join(dir, "dnd-home")
}

func applyYes(t *testing.T, p *ConfigPlan) string {
	t.Helper()
	var out strings.Builder
	ok, err := p.Apply(Confirm{Out: &out, Yes: true, Now: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)})
	if err != nil || !ok {
		t.Fatalf("apply: ok=%v err=%v\n%s", ok, err, out.String())
	}
	return out.String()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCodexInstallNoNotify(t *testing.T) {
	path, home := codexFixture(t, userConfig)
	p, err := PlanCodexInstall(path, testDnd, home)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(userConfig, "sandbox_mode = 'workspace-write'\n",
		"sandbox_mode = 'workspace-write'\nnotify = [\"/u/go/bin/dnd\", \"hook\", \"codex\"]\n", 1)
	if string(p.New) != want {
		t.Fatalf("got:\n%s", p.New)
	}
	if !strings.Contains(p.Diff(), "+notify = [\"/u/go/bin/dnd\", \"hook\", \"codex\"]") {
		t.Fatalf("diff:\n%s", p.Diff())
	}
	out := applyYes(t, p)
	if !strings.Contains(out, "已備份到 "+path+".dnd-backup-20261104-090000") {
		t.Fatal(out)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
		t.Fatalf("權限應保留: %v", fi.Mode())
	}
	if _, err := os.Stat(OriginalNotifyPath(home)); err == nil {
		t.Fatal("沒有原本的 notify 時不應建立串接檔")
	}

	// 再裝一次：不需要改；換路徑：只改路徑
	if p, _ := PlanCodexInstall(path, testDnd, home); p.Changed() {
		t.Fatalf("重複安裝不應改動:\n%s", p.Diff())
	}
	p, err = PlanCodexInstall(path, "/opt/bin/dnd", home)
	if err != nil || !strings.Contains(string(p.New), `notify = ["/opt/bin/dnd", "hook", "codex"]`) || p.saveOriginal != nil {
		t.Fatalf("%v\n%s", err, p.New)
	}

	// uninstall：回到原檔
	p, err = PlanCodexUninstall(path, home)
	if err != nil {
		t.Fatal(err)
	}
	applyYes(t, p)
	if got := readFile(t, path); got != userConfig {
		t.Fatalf("uninstall 應還原原檔:\n%s", got)
	}
	if p, _ := PlanCodexUninstall(path, home); p.Changed() {
		t.Fatal("沒裝時 uninstall 不應改動")
	}
}

func TestCodexInstallChainsOriginal(t *testing.T) {
	orig := strings.Replace(userConfig, "approval_policy", "notify = [\n  \"python3\",\n  \"/home/me/notify.py\", # 桌面通知\n]\napproval_policy", 1)
	path, home := codexFixture(t, orig)
	p, err := PlanCodexInstall(path, testDnd, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Notes) != 1 || !strings.Contains(p.Notes[0], "python3 /home/me/notify.py") {
		t.Fatalf("notes: %q", p.Notes)
	}
	want := strings.Replace(userConfig, "approval_policy", "notify = [\"/u/go/bin/dnd\", \"hook\", \"codex\"]\napproval_policy", 1)
	if string(p.New) != want {
		t.Fatalf("got:\n%s", p.New)
	}
	applyYes(t, p)
	argv, err := ReadOriginalNotify(home)
	if err != nil || !slices.Equal(argv, []string{"python3", "/home/me/notify.py"}) {
		t.Fatalf("%q %v", argv, err)
	}

	// 已經串接時再裝：不改，也不覆寫串接檔
	if p, _ := PlanCodexInstall(path, testDnd, home); p.Changed() || p.saveOriginal != nil {
		t.Fatal("重複安裝不應改動")
	}

	p, err = PlanCodexUninstall(path, home)
	if err != nil || len(p.Notes) != 1 {
		t.Fatalf("%v %q", err, p.Notes)
	}
	applyYes(t, p)
	if got := readFile(t, path); got != orig {
		t.Fatalf("應逐字還原原本的 notify（含註解）:\n%s", got)
	}
	if _, err := os.Stat(OriginalNotifyPath(home)); !os.IsNotExist(err) {
		t.Fatal("還原後應刪除串接檔")
	}
}

// 串接檔的原文對不上時，改用陣列還原。
func TestCodexUninstallFallsBackToArray(t *testing.T) {
	path, home := codexFixture(t, "notify = [\"/x/dnd\", \"hook\", \"codex\"]\n")
	os.MkdirAll(filepath.Dir(OriginalNotifyPath(home)), 0o755)
	os.WriteFile(OriginalNotifyPath(home), []byte(`{"notify":["say","done"],"toml":"[\"other\"]"}`), 0o644)
	p, err := PlanCodexUninstall(path, home)
	if err != nil || string(p.New) != "notify = [\"say\", \"done\"]\n" {
		t.Fatalf("%v\n%s", err, p.New)
	}
	os.WriteFile(OriginalNotifyPath(home), []byte(`{`), 0o644)
	if _, err := PlanCodexUninstall(path, home); err == nil {
		t.Fatal("壞掉的串接檔應回報錯誤")
	}
}

func TestCodexInstallShapes(t *testing.T) {
	notify := `notify = ["/u/go/bin/dnd", "hook", "codex"]`
	for name, c := range map[string]struct{ in, want string }{
		"不存在":        {"", notify + "\n"},
		"空檔":         {"\n", notify + "\n\n"},
		"只有 table":   {"[profiles.x]\nmodel = \"a\"\n", notify + "\n[profiles.x]\nmodel = \"a\"\n"},
		"結尾沒有換行":     {`model = "a"`, "model = \"a\"\n" + notify},
		"CRLF":       {"model = \"a\"\r\n\r\n[t]\r\nk = 1\r\n", "model = \"a\"\r\n" + notify + "\r\n\r\n[t]\r\nk = 1\r\n"},
		"空陣列":        {"notify = []\nmodel = \"a\"\n", notify + "\nmodel = \"a\"\n"},
		"引號 key":     {"\"notify\" = [\"a\"] # x\n", "\"notify\" = [\"/u/go/bin/dnd\", \"hook\", \"codex\"] # x\n"},
		"table 裡的同名": {"[profiles.x]\nnotify = [\"a\"]\n", notify + "\n[profiles.x]\nnotify = [\"a\"]\n"},
		"多行字串": {"instructions = \"\"\"\nnotify = [\"x\"]\n[fake]\n\"\"\"\n[t]\n",
			"instructions = \"\"\"\nnotify = [\"x\"]\n[fake]\n\"\"\"\n" + notify + "\n[t]\n"},
		"BOM": {"\xef\xbb\xbf[t]\n", "\xef\xbb\xbf" + notify + "\n[t]\n"},
	} {
		t.Run(name, func(t *testing.T) {
			path, home := codexFixture(t, c.in)
			p, err := PlanCodexInstall(path, testDnd, home)
			if err != nil {
				t.Fatal(err)
			}
			if string(p.New) != c.want {
				t.Fatalf("got:\n%q\nwant:\n%q", p.New, c.want)
			}
			applyYes(t, p)
			p, err = PlanCodexUninstall(path, home)
			if err != nil {
				t.Fatal(err)
			}
			applyYes(t, p)
			got := readFile(t, path)
			if c.in == "notify = []\nmodel = \"a\"\n" {
				c.in = "model = \"a\"\n" // 空陣列等於沒設定，uninstall 直接刪掉
			}
			if got != c.in {
				t.Fatalf("roundtrip:\n%q\nwant:\n%q", got, c.in)
			}
		})
	}
}

func TestCodexInstallRejects(t *testing.T) {
	for name, in := range map[string]string{
		"壞掉的 TOML":    "model = \n",
		"notify 不是陣列": "notify = \"say\"\n",
		"notify 有數字":  "notify = [\"say\", 1]\n",
	} {
		t.Run(name, func(t *testing.T) {
			path, home := codexFixture(t, in)
			if _, err := PlanCodexInstall(path, testDnd, home); err == nil {
				t.Fatal("應回報錯誤")
			}
			if _, err := PlanCodexUninstall(path, home); err == nil {
				t.Fatal("應回報錯誤")
			}
			if readFile(t, path) != in {
				t.Fatal("不可改檔")
			}
		})
	}
}

func TestIsDndNotify(t *testing.T) {
	for argv, want := range map[string]bool{
		"/u/go/bin/dnd hook codex":       true,
		"dnd.exe hook codex":             true,
		"/u/go/bin/dnd hook codex extra": false,
		"/u/go/bin/dnd hook claude":      false,
		"/u/bin/notdnd hook codex":       false,
		"":                               false,
	} {
		if got := IsDndNotify(strings.Fields(argv)); got != want {
			t.Errorf("%q: %v", argv, got)
		}
	}
}

func TestCodexConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	if p, _ := CodexConfigPath(""); p != filepath.Join(dir, "config.toml") {
		t.Fatal(p)
	}
	if p, _ := CodexConfigPath("/x/c.toml"); p != "/x/c.toml" {
		t.Fatal(p)
	}
}

func TestCodexConfigStatus(t *testing.T) {
	path, home := codexFixture(t, "")
	if st, err := CodexConfigStatus(path, home); err != nil || st.Exists {
		t.Fatalf("%+v %v", st, err)
	}
	os.WriteFile(path, []byte(userConfig), 0o600)
	if st, err := CodexConfigStatus(path, home); err != nil || !st.Exists || st.Installed {
		t.Fatalf("%+v %v", st, err)
	}
	os.WriteFile(path, []byte("notify = [\"/u/go/bin/dnd\", \"hook\", \"codex\"]\n"), 0o600)
	os.MkdirAll(filepath.Dir(OriginalNotifyPath(home)), 0o755)
	os.WriteFile(OriginalNotifyPath(home), []byte(`{"notify":["say","hi"]}`), 0o644)
	st, err := CodexConfigStatus(path, home)
	if err != nil || !st.Installed || st.DndPath != testDnd || !slices.Equal(st.Original, []string{"say", "hi"}) {
		t.Fatalf("%+v %v", st, err)
	}
	os.WriteFile(path, []byte("notify = "), 0o600)
	if _, err := CodexConfigStatus(path, home); err == nil {
		t.Fatal("壞掉的 TOML 應回報錯誤")
	}
}

// 任何合法的 TOML 都不能讓 installer panic，掃描器也要找得到正確的位置；成功時 install → uninstall 要回到原檔。
func FuzzCodexInstall(f *testing.F) {
	for _, s := range []string{userConfig, "notify = [\"a\", 'b']\n", "a.b = 1\n[[x]]\ny = \"\"\"\n]\n\"\"\"\n", "k = { a = [1, 2] } # c\n", ""} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		if _, err := decodeTOML(in); err != nil || len(in) == 0 {
			return
		}
		path, home := codexFixture(t, string(in))
		p, err := PlanCodexInstall(path, testDnd, home)
		if err != nil && (strings.Contains(err.Error(), "檢查沒有通過") || strings.Contains(err.Error(), "讀不懂")) {
			t.Fatalf("合法的 TOML 卻改不了: %v\n%q", err, in)
		}
		if err != nil || !p.Changed() {
			return
		}
		applyYes(t, p)
		if p, err = PlanCodexUninstall(path, home); err != nil {
			t.Fatalf("uninstall: %v\n%q", err, readFile(t, path))
		}
		applyYes(t, p)
		if got := readFile(t, path); got != string(in) && !strings.Contains(string(in), "notify") {
			t.Fatalf("roundtrip:\n%q\n%q", in, got)
		}
	})
}
