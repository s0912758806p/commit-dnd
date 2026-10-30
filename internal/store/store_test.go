package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

var t0 = time.Date(2026, 10, 30, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

func open(t *testing.T, dir string) *FileStore {
	t.Helper()
	p, err := content.Default()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, p)
	if err != nil {
		t.Fatal(err)
	}
	s.LockTimeout = 10 * time.Second
	return s
}

func pet(i int) event.Event {
	return event.Event{At: t0.Add(time.Duration(i) * 7 * time.Minute), Source: event.SourceUser, Type: "pet"}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func countLines(t *testing.T, dir string) int {
	return bytes.Count(readFile(t, filepath.Join(dir, eventsFile)), []byte("\n"))
}

func TestUpdateAssignsSeqAndPersists(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := range 3 {
		r, err := s.Update(pet(i))
		if err != nil {
			t.Fatal(err)
		}
		if r.State.LastSeq != int64(i+1) {
			t.Fatalf("LastSeq = %d", r.State.LastSeq)
		}
	}
	st, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSeq != 3 || st.Pet.Pets != 3 {
		t.Fatalf("Load = %+v", st)
	}

	var first event.Event
	line, _, _ := bytes.Cut(readFile(t, filepath.Join(dir, eventsFile)), []byte("\n"))
	if err := json.Unmarshal(line, &first); err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || first.V != event.Version || first.At.Location() != time.UTC {
		t.Fatalf("第一筆事件 = %+v", first)
	}
}

func TestLoadWithoutData(t *testing.T) {
	st, err := open(t, t.TempDir()).Load()
	if err != nil || st.V != 0 {
		t.Fatalf("st=%+v err=%v", st, err)
	}
}

// 迭代 0 的驗收：刪掉 state.json 後 Rebuild 得到逐位元組相同的 state.json。
func TestRebuildAfterDeletingState(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := range 5 {
		if _, err := s.Update(pet(i)); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(dir, stateFile)
	want := readFile(t, statePath)
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, statePath); !bytes.Equal(got, want) {
		t.Fatalf("重建後不同:\n got %s\nwant %s", got, want)
	}
}

// 並行的 hook 不能互相蓋掉：每個 goroutine 用自己的 FileStore（各自的檔案鎖）。
func TestConcurrentUpdatesLoseNothing(t *testing.T) {
	dir := t.TempDir()
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := open(t, dir).Update(pet(i))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	st, err := open(t, dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.LastSeq != n || st.Pet.Pets != n || countLines(t, dir) != n {
		t.Fatalf("LastSeq=%d Pets=%d lines=%d", st.LastSeq, st.Pet.Pets, countLines(t, dir))
	}
}

// 模擬「追加事件後、寫 state.json 前」當機：state.json 落後，下一次 Update 要先補上。
func TestUpdateCatchesUpStaleState(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := s.Update(pet(0)); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, stateFile)
	stale := readFile(t, statePath)
	if _, err := s.Update(pet(1)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := s.Update(pet(2))
	if err != nil {
		t.Fatal(err)
	}
	if r.State.LastSeq != 3 || r.State.Pet.Pets != 3 {
		t.Fatalf("沒有補上落後的狀態: %+v", r.State)
	}
}

// 當機留下半行：重播略過它並截掉，之後的追加不會黏在半行後面。
func TestTornTailIsTruncated(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	for i := range 2 {
		if _, err := s.Update(pet(i)); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"v":1,"seq":3,"at":"2026-`)
	f.Close()

	r, err := s.Update(pet(2))
	if err != nil {
		t.Fatal(err)
	}
	if r.State.LastSeq != 3 || countLines(t, dir) != 3 {
		t.Fatalf("LastSeq=%d lines=%d", r.State.LastSeq, countLines(t, dir))
	}
	if _, err := s.Rebuild(); err != nil {
		t.Fatalf("截斷後應可完整重播: %v", err)
	}
}

func TestCorruptMiddleLineIsAnError(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if _, err := s.Update(pet(0)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, eventsFile)
	data := append([]byte("garbage\n"), readFile(t, path)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebuild(); err == nil {
		t.Fatal("中間行損毀不可默默丟掉")
	}
}

func TestLockTimeout(t *testing.T) {
	dir := t.TempDir()
	holder := open(t, dir)
	unlock, err := holder.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	s := open(t, dir)
	s.LockTimeout = 30 * time.Millisecond
	if _, err := s.Update(pet(0)); err != ErrBusy {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}
