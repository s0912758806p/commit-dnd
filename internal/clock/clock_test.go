package clock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOffsetOnlyInDebugMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DND_HOME", dir)
	if err := SetOffset(dir, 8*time.Hour); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DND_DEBUG", "")
	if d := time.Until(Now()); d > time.Minute {
		t.Fatalf("未開 DND_DEBUG 時偏移不應生效：%v", d)
	}
	t.Setenv("DND_DEBUG", "0")
	if d := time.Until(Now()); d > time.Minute {
		t.Fatalf("DND_DEBUG=0 時偏移不應生效：%v", d)
	}
	t.Setenv("DND_DEBUG", "1")
	if d := time.Until(Now()); d < 8*time.Hour-time.Minute || d > 8*time.Hour+time.Minute {
		t.Fatalf("DND_DEBUG=1 時應快轉 8 小時：%v", d)
	}
}

func TestOffsetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if d, err := Offset(dir); err != nil || d != 0 {
		t.Fatalf("沒有檔案應為 0：%v %v", d, err)
	}
	if err := SetOffset(dir, -90*time.Minute); err != nil {
		t.Fatal(err)
	}
	if d, err := Offset(dir); err != nil || d != -90*time.Minute {
		t.Fatalf("got %v %v", d, err)
	}
	if err := SetOffset(dir, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, OffsetFile)); !os.IsNotExist(err) {
		t.Fatalf("偏移 0 應刪檔：%v", err)
	}
}

// 檔案壞掉時 Offset 回報錯誤，但 Now 照常回傳真實時間（hook 不能因此失敗）。
func TestCorruptOffsetFallsBackToRealTime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DND_HOME", dir)
	t.Setenv("DND_DEBUG", "1")
	os.WriteFile(filepath.Join(dir, OffsetFile), []byte("八小時"), 0o644)
	if _, err := Offset(dir); err == nil {
		t.Fatal("壞掉的內容應報錯")
	}
	if d := time.Until(Now()); d > time.Minute || d < -time.Minute {
		t.Fatalf("應回傳真實時間：%v", d)
	}
}

func TestFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{8 * time.Hour: "+8h0m0s", -time.Hour: "-1h0m0s", 0: "0s"} {
		if got := Format(d); got != want {
			t.Errorf("Format(%v) = %q, want %q", d, got, want)
		}
	}
}
