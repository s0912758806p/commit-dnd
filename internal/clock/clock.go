// Package clock 提供 CLI 用的「現在」：真實時間加上 `dnd debug time` 設定的偏移。
//
// 引擎本身不讀時鐘；所有產生事件（hook、event、hatch、warp…）或在記憶體套用 tick（status、statusline、dungeon）
// 的地方都改呼叫 Now，偏移因此一致地套到每一個「現在」。偏移存在 $DND_HOME/debug-time-offset
// （Go 的 duration 字串，例如 "8h0m0s"），**只在 DND_DEBUG=1 時生效**：正式使用時就算檔案殘留也讀不到，
// `dnd doctor` 會提醒殘留檔。讀檔失敗或內容壞掉時視為沒有偏移，hook 不會因此失敗。
package clock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/home"
)

// OffsetFile 是 DND_HOME 底下存偏移的檔名。
const OffsetFile = "debug-time-offset"

// Enabled 回報除錯模式（DND_DEBUG=1）是否開啟；只有開啟時偏移才生效。
func Enabled() bool { return os.Getenv("DND_DEBUG") == "1" }

// Now 回傳真實時間；除錯模式下加上 DND_HOME 裡的偏移。
func Now() time.Time {
	now := time.Now()
	if !Enabled() {
		return now
	}
	dir, err := home.Dir()
	if err != nil {
		return now
	}
	off, err := Offset(dir)
	if err != nil {
		return now
	}
	return now.Add(off)
}

// Offset 讀取 dir 裡的偏移；沒有檔案時為 0。不看 DND_DEBUG，給 debug time 與 doctor 顯示用。
func Offset(dir string) (time.Duration, error) {
	b, err := os.ReadFile(filepath.Join(dir, OffsetFile))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("%s 內容壞掉：%w", OffsetFile, err)
	}
	return d, nil
}

// SetOffset 寫入偏移；d 為 0 時刪除檔案。
func SetOffset(dir string, d time.Duration) error {
	p := filepath.Join(dir, OffsetFile)
	if d == 0 {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(d.String()+"\n"), 0o644)
}

// Format 把偏移寫成帶正負號的形式（+8h0m0s、-1h0m0s、0s）。
func Format(d time.Duration) string {
	if d > 0 {
		return "+" + d.String()
	}
	return d.String()
}
