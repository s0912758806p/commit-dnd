package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/store"
)

var update = flag.Bool("update", false, "用目前輸出覆寫 testdata/golden")

// debugHome 建立暫存 DND_HOME、開啟 DND_DEBUG 並孵一隻寵物。
func debugHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DND_HOME", dir)
	t.Setenv("DND_DEBUG", "1")
	if _, err := dnd(t, "hatch", "--species", "elf", "--name", "小精"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustDnd(t *testing.T, args ...string) string {
	t.Helper()
	out, err := dnd(t, args...)
	if err != nil {
		t.Fatalf("dnd %v: %v\n%s", args, err, out)
	}
	return out
}

func testStore(t *testing.T, dir string) *store.FileStore {
	t.Helper()
	pack, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(dir, pack)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func loadState(t *testing.T, dir string) engine.State {
	t.Helper()
	st, err := testStore(t, dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func readEvents(t *testing.T, dir string) []event.Event {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []event.Event
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		var e event.Event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}

func lastEvent(t *testing.T, dir string) event.Event {
	t.Helper()
	events := readEvents(t, dir)
	return events[len(events)-1]
}

// near 檢查 at 與「現在 + want」相差不到一分鐘。
func near(t *testing.T, at time.Time, want time.Duration) {
	t.Helper()
	if d := time.Until(at) - want; d < -time.Minute || d > time.Minute {
		t.Fatalf("事件時間 %v 應約為現在 %+v", at, want)
	}
}

func TestDebugOnlyWithFlag(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	t.Setenv("DND_DEBUG", "")
	for _, args := range [][]string{{"debug", "time"}, {"debug", "xp", "10"}, {"debug", "stats"}} {
		if _, err := dnd(t, args...); err == nil {
			t.Fatalf("未設 DND_DEBUG=1 時 %v 不應存在", args)
		}
	}
}

func TestDebugTime(t *testing.T) {
	dir := debugHome(t)
	if out := mustDnd(t, "debug", "time"); !strings.HasPrefix(out, "時間偏移 0s") {
		t.Fatalf("%q", out)
	}
	mustDnd(t, "debug", "time", "+8h")
	if out := mustDnd(t, "debug", "time", "-1h"); !strings.HasPrefix(out, "時間偏移 +7h0m0s") {
		t.Fatalf("偏移應累加: %q", out)
	}
	if out := mustDnd(t, "debug", "time"); !strings.HasPrefix(out, "時間偏移 +7h0m0s") {
		t.Fatalf("%q", out)
	}
	for _, bad := range [][]string{{"debug", "time", "八小時"}, {"debug", "time", "+1h", "+2h"}} {
		if _, err := dnd(t, bad...); err == nil {
			t.Fatalf("%v 應報錯", bad)
		}
	}

	// 產生事件（event、hook）與套用 tick（status）都用偏移後的現在
	mustDnd(t, "event", "user", "pet")
	near(t, lastEvent(t, dir).At, 7*time.Hour)
	root := newRoot()
	root.SetIn(strings.NewReader(`{"hook_event_name":"Stop","cwd":"/x/repo"}`))
	root.SetArgs([]string{"hook", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	near(t, lastEvent(t, dir).At, 7*time.Hour)

	// 正式使用（沒有 DND_DEBUG=1）時殘留的偏移檔不生效，doctor 會提醒
	t.Setenv("DND_DEBUG", "")
	mustDnd(t, "event", "user", "pet")
	near(t, lastEvent(t, dir).At, 0)
	out, _ := dnd(t, "doctor", "--repo", t.TempDir(), "--settings", filepath.Join(t.TempDir(), "settings.json"))
	if !strings.Contains(out, "! 除錯時間偏移：殘留 +7h0m0s") {
		t.Fatalf("doctor 應提醒殘留的偏移檔:\n%s", out)
	}

	t.Setenv("DND_DEBUG", "1")
	mustDnd(t, "debug", "time", "reset")
	if _, err := os.Stat(filepath.Join(dir, clock.OffsetFile)); !os.IsNotExist(err) {
		t.Fatalf("reset 應刪掉偏移檔: %v", err)
	}
	out, _ = dnd(t, "doctor", "--repo", t.TempDir(), "--settings", filepath.Join(t.TempDir(), "settings.json"))
	if strings.Contains(out, "除錯時間偏移") {
		t.Fatalf("沒有偏移時 doctor 不應提起:\n%s", out)
	}
}

// 快轉 8 小時：飽足照偏移後的現在下降，被打掉的 HP 回滿。
func TestDebugTimeAdvancesTimeEffects(t *testing.T) {
	dir := debugHome(t)
	mustDnd(t, "debug", "floor", "1")
	for range 2 {
		mustDnd(t, "event", "claude", "turn") // 第一回合遇到怪物，第二回合被打
	}
	pack, _ := content.Default()
	before := loadState(t, dir)
	if before.Pet.HP >= pack.MaxHP(before.Pet.Level) || before.Dungeon.Downed {
		t.Fatalf("前置條件：應被打傷但沒倒地 %+v %+v", before.Pet, before.Dungeon)
	}
	mustDnd(t, "debug", "time", "+8h")
	mustDnd(t, "event", "user", "pet")
	after := loadState(t, dir)
	if after.Pet.Satiety >= before.Pet.Satiety || after.Pet.HP != pack.MaxHP(after.Pet.Level) {
		t.Fatalf("飽足 %d → %d，HP %d → %d", before.Pet.Satiety, after.Pet.Satiety, before.Pet.HP, after.Pet.HP)
	}
}

func TestDebugXPAndFloor(t *testing.T) {
	dir := debugHome(t)
	if out := mustDnd(t, "debug", "xp", "5000"); !strings.Contains(out, "獲得 5000 XP") || !strings.Contains(out, "升到了 Lv 2") {
		t.Fatalf("%s", out)
	}
	if out := mustDnd(t, "debug", "floor", "40"); !strings.Contains(out, "B40") {
		t.Fatalf("%s", out)
	}
	st := loadState(t, dir)
	if st.Pet.Level < 10 || st.Dungeon.Floor != 40 || st.Dungeon.Deepest != 40 {
		t.Fatalf("Lv %d %+v", st.Pet.Level, st.Dungeon)
	}
	if out := mustDnd(t, "dungeon"); !strings.HasPrefix(out, "樓層  B40") {
		t.Fatalf("%s", out)
	}
	for _, bad := range [][]string{{"debug", "xp", "0"}, {"debug", "xp", "abc"}, {"debug", "floor", "-1"}, {"debug", "floor"}} {
		if _, err := dnd(t, bad...); err == nil {
			t.Fatalf("%v 應報錯", bad)
		}
	}
	// 除錯事件留在 events.jsonl，重播結果相同
	rebuilt, err := testStore(t, dir).Rebuild()
	if err != nil || rebuilt != st {
		t.Fatalf("Rebuild 不一致: %v\n%+v\n%+v", err, rebuilt, st)
	}
}

func TestDebugEvent(t *testing.T) {
	dir := debugHome(t)
	out := mustDnd(t, "debug", "event", "git", "commit", "kind=fix", "added=10", "--count", "3")
	if strings.Count(out, "git/commit") != 3 {
		t.Fatalf("%s", out)
	}
	if st := loadState(t, dir); st.Pet.Commits != 3 {
		t.Fatalf("三筆 commit 都要計分（hash 不重複）: %d", st.Pet.Commits)
	}
	e := lastEvent(t, dir)
	if e.Commit == nil || e.Commit.Kind != "fix" || e.Commit.Added != 10 || !strings.HasPrefix(e.Commit.Hash, "debug-") || e.Args != nil {
		t.Fatalf("%+v %+v", e, e.Commit)
	}

	// dnd event 與 dnd debug event 是同一個指令；tool= 填進 Event.Tool
	mustDnd(t, "event", "claude", "tool_use", "tool=Edit", "repo=demo", "note=x")
	if e := lastEvent(t, dir); e.Tool != "Edit" || e.Repo != "demo" || e.Args["note"] != "x" || e.Commit != nil {
		t.Fatalf("%+v", e)
	}
	mustDnd(t, "debug", "event", "git", "commit", "hash=abc")
	if out := mustDnd(t, "debug", "event", "git", "commit", "hash=abc"); strings.Contains(out, "XP") {
		t.Fatalf("同一個 hash 不應重複計分:\n%s", out)
	}

	n := len(readEvents(t, dir))
	for _, bad := range [][]string{
		{"debug", "event", "git", "commit", "added=很多"},
		{"debug", "event", "git", "commit", "deleted=-1"},
		{"debug", "event", "nobody", "turn"},
		{"debug", "event", "claude", "turn", "oops"},
		{"debug", "event", "claude", "turn", "--count", "0"},
	} {
		if _, err := dnd(t, bad...); err == nil {
			t.Fatalf("%v 應報錯", bad)
		}
	}
	if got := len(readEvents(t, dir)); got != n {
		t.Fatalf("參數錯誤時不應寫入事件: %d → %d", n, got)
	}
}

func TestDebugSnapshot(t *testing.T) {
	dir := debugHome(t)
	mustDnd(t, "debug", "event", "git", "commit", "kind=feat")
	mustDnd(t, "debug", "time", "+2h")
	saved := loadState(t, dir)
	if out := mustDnd(t, "debug", "snapshot", "save", "start"); !strings.Contains(out, "2 個事件") {
		t.Fatalf("%s", out)
	}
	if _, err := dnd(t, "debug", "snapshot", "save", "start"); err == nil {
		t.Fatal("同名快照應要求 --force")
	}

	mustDnd(t, "debug", "time", "reset")
	mustDnd(t, "debug", "xp", "1000")
	mustDnd(t, "debug", "floor", "20")
	out := mustDnd(t, "debug", "snapshot", "load", "start")
	if !strings.Contains(out, "before-load-") {
		t.Fatalf("%s", out)
	}

	// 載回後狀態、事件、偏移都和存檔時相同，Rebuild 也相同
	if st := loadState(t, dir); st != saved {
		t.Fatalf("載回的狀態不同:\n%+v\n%+v", st, saved)
	}
	if rebuilt, err := testStore(t, dir).Rebuild(); err != nil || rebuilt != saved {
		t.Fatalf("Rebuild 不一致: %v\n%+v\n%+v", err, rebuilt, saved)
	}
	if off, _ := clock.Offset(dir); off != 2*time.Hour {
		t.Fatalf("偏移也要載回: %v", off)
	}

	// 載入前的資料另存了一份，載回它就回到載入前
	backups, _ := filepath.Glob(filepath.Join(dir, "snapshots", "before-load-*"))
	if len(backups) != 1 {
		t.Fatalf("%v", backups)
	}
	mustDnd(t, "debug", "snapshot", "load", filepath.Base(backups[0]))
	if st := loadState(t, dir); st.Dungeon.Floor != 20 {
		t.Fatalf("%+v", st.Dungeon)
	}
	if _, err := os.Stat(filepath.Join(dir, clock.OffsetFile)); !os.IsNotExist(err) {
		t.Fatalf("載入前沒有偏移，載回後也不應有: %v", err)
	}

	out = mustDnd(t, "debug", "snapshot", "list")
	if !strings.Contains(out, "start  2 個事件") || strings.Count(out, "before-load-") != 2 {
		t.Fatalf("%s", out)
	}
	for _, bad := range [][]string{
		{"debug", "snapshot", "load", "nope"},
		{"debug", "snapshot", "save", "../x"},
		{"debug", "snapshot", "save", ".."},
		{"debug", "snapshot", "load", "a/b"},
	} {
		if _, err := dnd(t, bad...); err == nil {
			t.Fatalf("%v 應報錯", bad)
		}
	}
}

func TestDebugSnapshotListEmpty(t *testing.T) {
	debugHome(t)
	if out := mustDnd(t, "debug", "snapshot", "list"); !strings.Contains(out, "還沒有快照") {
		t.Fatalf("%q", out)
	}
}

// stats 的輸出由 golden 檔守住：兩天的 commit、AI 回合、攻擊、跳樓層與倒地。
func TestDebugStats(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DND_HOME", dir)
	t.Setenv("DND_DEBUG", "1")
	if out := mustDnd(t, "debug", "stats"); out != "events.jsonl 沒有事件。\n" {
		t.Fatalf("%q", out)
	}

	cst := time.FixedZone("CST", 8*3600)
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, cst)
	s := testStore(t, dir)
	put := func(e event.Event) {
		t.Helper()
		e.At = at
		at = at.Add(time.Minute)
		if _, err := s.Update(e); err != nil {
			t.Fatal(err)
		}
	}
	put(event.Event{Source: event.SourceUser, Type: "hatch", Args: map[string]string{"species": "elf", "name": "小精"}})
	for i, kind := range []string{"feat", "fix"} {
		put(event.Event{Source: event.SourceGit, Type: "commit", Commit: &event.CommitInfo{Kind: kind, Added: 20, Hash: "h" + string(rune('a'+i))}})
	}
	for range 3 { // 每回合：下探或遇怪，再用 Edit 打一下
		put(event.Event{Source: event.SourceClaude, Type: "turn"})
		for range 4 {
			put(event.Event{Source: event.SourceClaude, Type: "tool_use", Tool: "Edit"})
		}
	}

	at = time.Date(2026, 11, 2, 23, 0, 0, 0, cst)
	put(event.Event{Source: event.SourceUser, Type: "debug_floor", Args: map[string]string{"floor": "30"}})
	for range 20 { // B30 的怪物打到倒地
		put(event.Event{Source: event.SourceClaude, Type: "turn"})
	}
	at = time.Date(2026, 11, 3, 8, 0, 0, 0, cst)
	put(event.Event{Source: event.SourceGit, Type: "commit", Commit: &event.CommitInfo{Kind: "fix", Added: 3, Hash: "hz"}})
	put(event.Event{Source: event.SourceUser, Type: "debug_xp", Args: map[string]string{"xp": "100"}})

	if st := loadState(t, dir); st.Dungeon.Downs != 1 {
		t.Fatalf("前置條件：B30 應倒地一次 %+v", st.Dungeon)
	}
	golden(t, "debug_stats.txt", mustDnd(t, "debug", "stats"))
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v（第一次請加 -update）", err)
	}
	if got != string(want) {
		t.Fatalf("%s 不符：\n--- got\n%s--- want\n%s", name, got, want)
	}
}
