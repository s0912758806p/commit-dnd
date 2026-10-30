package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

var now = time.Date(2026, 10, 30, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

// testdata/*.txt 是在真的 repo 跑 Command 收集來的輸出。
func TestParseSamples(t *testing.T) {
	cases := map[string]*event.CommitInfo{
		"initial.txt":              {Kind: "chore", Added: 2, Hash: "b85dfce75aaa57bb234deb703883b85e2029842a"},
		"feat_lockfile_binary.txt": {Kind: "feat", Added: 3, Hash: "915007841a71951ec17a80fae59f548e8dddda36"},
		"plain_subject.txt":        {Kind: "other", Added: 1, Hash: "a86826989527449b61f2c6e9fec4f476f038bb92"},
		"amend.txt":                nil,
		"cherry_pick.txt":          nil,
		"rebase.txt":               nil,
		"checkout.txt":             nil,
		"reset.txt":                nil,
	}
	for file, want := range cases {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			evs, err := Adapter{Repo: "demo"}.Parse(raw, now)
			if err != nil {
				t.Fatal(err)
			}
			if want == nil {
				if len(evs) != 0 {
					t.Fatalf("不應計分: %+v", evs[0].Commit)
				}
				return
			}
			if len(evs) != 1 {
				t.Fatalf("events = %d", len(evs))
			}
			e := evs[0]
			if *e.Commit != *want || e.Type != "commit" || e.Source != event.SourceGit || e.Repo != "demo" || !e.At.Equal(now) {
				t.Fatalf("got %+v commit=%+v", e, e.Commit)
			}
		})
	}
}

func TestKind(t *testing.T) {
	for subject, want := range map[string]string{
		"feat: x": "feat", "Fix(core): y": "fix", "refactor!: z": "refactor", "feat(a)!: b": "feat",
		"Update readme": "other", "feat x": "other", "": "other", "WIP: stuff": "wip",
	} {
		if got := Kind(subject); got != want {
			t.Errorf("Kind(%q) = %q, want %q", subject, got, want)
		}
	}
}

func TestParseGarbage(t *testing.T) {
	for _, raw := range []string{"", "\n", "abc", "\x1e", "a\x1fb\x1e", "h\x1fcommit: x\x1fx\x1e\n1\t2", "h\x1fcommit: x\x1fx\x1e\n-5\t3\tf\nx\ty\tz", "h\x1fcommit: x\x1fx\x1e\n9223372036854775807\t0\ta\n9223372036854775807\t0\tb"} {
		evs, err := Adapter{}.Parse([]byte(raw), now)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		for _, e := range evs {
			if e.Commit.Added < 0 || e.Commit.Deleted < 0 {
				t.Fatalf("%q: 負的行數 %+v", raw, e.Commit)
			}
		}
	}
}

// 任何輸入都不能讓 hook 失敗：Parse 不可 panic，也不可回傳負的行數。
func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.txt"))
	for _, file := range files {
		raw, _ := os.ReadFile(file)
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		evs, _ := Adapter{}.Parse(raw, now)
		for _, e := range evs {
			if e.Commit == nil || e.Commit.Added < 0 || e.Commit.Deleted < 0 {
				t.Fatalf("bad event %+v", e)
			}
		}
	})
}
