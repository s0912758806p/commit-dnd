// Package adapter 定義「外部輸入 → event.Event」的共同介面；各來源在子套件實作。
//
// Parse 不讀時鐘：now 由呼叫端傳入，所以 adapter 可以用 testdata 樣本做決定性測試。
// 任何輸入（壞掉的、空的、意料之外的）都不可以 panic；不認得的輸入回傳零個事件。
package adapter

import (
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

type Adapter interface {
	Parse(raw []byte, now time.Time) ([]event.Event, error)
}
