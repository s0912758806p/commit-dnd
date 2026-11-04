package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

var now = time.Date(2026, 11, 4, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

// testdata/*.json 依 Codex 原始碼（codex-rs/hooks/src/legacy_notify.rs 的 UserNotification）
// 手寫並去識別化：agent_turn_complete 含選填的 client，minimal 沒有 client、
// last-assistant-message 為 null；unknown_type 是假想的其他 type。
func TestParseSamples(t *testing.T) {
	cases := map[string]bool{
		"agent_turn_complete.json":         true,
		"agent_turn_complete_minimal.json": true,
		"unknown_type.json":                false,
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
			if !want {
				if len(evs) != 0 {
					t.Fatalf("不應產生事件: %+v", evs)
				}
				return
			}
			if len(evs) != 2 {
				t.Fatalf("events = %d", len(evs))
			}
			for i, w := range []event.Event{{Type: "tool_use", Tool: "Codex"}, {Type: "turn"}} {
				e := evs[i]
				if e.Type != w.Type || e.Tool != w.Tool || e.Repo != "commit-dnd" || e.Source != event.SourceCodex || !e.At.Equal(now) || e.Args != nil {
					t.Fatalf("[%d] got %+v", i, e)
				}
			}
		})
	}
}

func TestParseMissingFields(t *testing.T) {
	for raw, n := range map[string]int{
		`{}`:                             0,
		`{"type":"agent-turn-complete"}`: 0, // 沒有 cwd
		`{"type":"agent-turn-complete","cwd":""}`:  0,
		`{"cwd":"/x/repo"}`:                        0, // 沒有 type
		`{"type":"agent-turn-complete","cwd":"/"}`: 2,
		`{"type":123}`:                             -1,
		`[]`:                                       -1,
		``:                                         -1,
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

// 任何輸入都不能讓 hook 失敗：Parse 不可 panic，產生的事件一定是「攻擊＋回合」一組。
func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.json"))
	for _, file := range files {
		raw, _ := os.ReadFile(file)
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		evs, _ := Adapter{}.Parse(raw, now)
		if len(evs) == 0 {
			return
		}
		if len(evs) != 2 || evs[0].Type != "tool_use" || evs[0].Tool != Tool || evs[1].Type != "turn" {
			t.Fatalf("bad events %+v", evs)
		}
	})
}
