package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

var now = time.Date(2026, 11, 4, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

// testdata/*.json：post_tool_use_*、stop 是 Claude Code 2.1.318 實際送給 hook 的 stdin，
// 經去識別化（保留欄位結構，字串換成假資料，cwd 一律為 /home/user/src/commit-dnd）；
// session_start.json 依官方文件格式手寫。
func TestParseSamples(t *testing.T) {
	cases := map[string]*event.Event{
		"post_tool_use_edit.json":  {Type: "tool_use", Tool: "Edit", Repo: "commit-dnd"},
		"post_tool_use_bash.json":  {Type: "tool_use", Tool: "Bash", Repo: "commit-dnd"},
		"post_tool_use_write.json": {Type: "tool_use", Tool: "Write", Repo: "commit-dnd"},
		"stop.json":                {Type: "turn", Repo: "commit-dnd"},
		"session_start.json":       nil,
	}
	for file, want := range cases {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			evs, err := Adapter{}.Parse(raw, now)
			if err != nil {
				t.Fatal(err)
			}
			if want == nil {
				if len(evs) != 0 {
					t.Fatalf("不應產生事件: %+v", evs)
				}
				return
			}
			if len(evs) != 1 {
				t.Fatalf("events = %d", len(evs))
			}
			e := evs[0]
			if e.Type != want.Type || e.Tool != want.Tool || e.Repo != want.Repo || e.Source != event.SourceClaude || !e.At.Equal(now) {
				t.Fatalf("got %+v", e)
			}
		})
	}
}

func TestParseMissingFields(t *testing.T) {
	for raw, n := range map[string]int{
		`{}`:                                 0,
		`{"hook_event_name":"PostToolUse"}`:  0, // 沒有 tool_name
		`{"hook_event_name":"Stop"}`:         1, // 沒有 cwd 也算回合
		`{"hook_event_name":"SubagentStop"}`: 0,
		`{"hook_event_name":123}`:            -1,
		`[]`:                                 -1,
		``:                                   -1,
		`{"hook_event_name":"PostToolUse","tool_name":"Read","cwd":"/"}`: 1,
	} {
		evs, err := Adapter{}.Parse([]byte(raw), now)
		if n < 0 {
			if err == nil {
				t.Errorf("%q: 應回傳錯誤", raw)
			}
			continue
		}
		if err != nil || len(evs) != n {
			t.Errorf("%q: %d 個事件, err=%v", raw, len(evs), err)
		}
	}
}

// 任何輸入都不能讓 hook 失敗：Parse 不可 panic。
func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.json"))
	for _, file := range files {
		raw, _ := os.ReadFile(file)
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		evs, _ := Adapter{}.Parse(raw, now)
		for _, e := range evs {
			if e.Type != "turn" && (e.Type != "tool_use" || e.Tool == "") {
				t.Fatalf("bad event %+v", e)
			}
		}
	})
}
