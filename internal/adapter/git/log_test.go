package git

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// testdata/log.txt 是在真的 repo 跑 LogArgs 收集來的輸出：含 lockfile、二進位檔，
// 以及一筆 rebase 過、作者時間比前一筆早的 commit（merge 已被 --no-merges 排除）。
func TestParseLog(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "log.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseLog(raw)
	var kinds []string
	for _, c := range got {
		kinds = append(kinds, c.Commit.Kind)
	}
	// 依作者時間（不同時區換算成同一時刻）由舊到新
	if want := []string{"docs", "other", "feat", "test"}; !slices.Equal(kinds, want) {
		t.Fatalf("kinds = %v", kinds)
	}
	if c := got[2].Commit; c.Added != 1 || c.Deleted != 0 || c.Hash != "5b3caf4dd24d05ff817dd057ca3ca23cec2fb353" {
		t.Fatalf("lockfile 與二進位檔不算行數: %+v", c)
	}
	if at := got[1].At; !at.Equal(time.Date(2026, 10, 1, 14, 0, 0, 0, time.FixedZone("", 8*3600))) || at.Format("-07:00") != "-07:00" {
		t.Fatalf("作者時間要保留原本的時區: %v", at)
	}
	if len(ParseLog(nil)) != 0 || len(ParseLog([]byte("\x1ebad\x1fnot-a-time\x1fx\n\x1e\x1f\x1f\n"))) != 0 {
		t.Fatal("格式不對的紀錄要略過")
	}
}

func TestLogArgs(t *testing.T) {
	if got := LogArgs("", ""); slices.ContainsFunc(got, func(a string) bool { return a == "--since=" || a == "--author=" }) {
		t.Fatalf("空字串不加條件: %v", got)
	}
	if got := LogArgs("2026-08-01T00:00:00+08:00", "me@example.com"); !slices.Contains(got, "--since=2026-08-01T00:00:00+08:00") || !slices.Contains(got, "--author=me@example.com") {
		t.Fatalf("%v", got)
	}
}
