package store

import (
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

// 給 `dnd debug` 用的兩個入口：snapshot 要在鎖內整批複製 DND_HOME 的檔案，stats 要逐筆看事件前後的狀態。

// WithLock 在寫入用的檔案鎖內執行 f；拿不到鎖時（逾時依 LockTimeout）回傳 ErrBusy，f 不會執行。
// f 裡不能再呼叫 Update／Rebuild（同一把鎖不可重入）。
func (s *FileStore) WithLock(f func() error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return f()
}

// Walk 從 events.jsonl 重播，對每個事件呼叫 visit(事件, 套用前的狀態, 結果)。不取鎖、不寫檔；
// 結尾的半行同 History 一樣略過。
func (s *FileStore) Walk(visit func(e event.Event, before engine.State, r engine.Result)) error {
	data, err := s.readEvents()
	if err != nil {
		return err
	}
	_, _, err = s.replayData(data, visit)
	return err
}
