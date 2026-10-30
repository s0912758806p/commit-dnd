// Package store 負責 DND_HOME 底下的兩個檔案：
//
//   - events.jsonl：唯一的事實來源，每行一個事件，只追加不修改。
//   - state.json：最新狀態的快取，可以隨時刪掉，由 Rebuild 從 events.jsonl 重播產生。
//
// Update 在同一把檔案鎖內完成「讀 state → 指派 Seq → engine.Apply → 追加事件 → 原子寫 state」，
// 所以並行的 hook 不會互相蓋掉。拿不到鎖會在 LockTimeout 後回傳 ErrBusy，由呼叫端決定是否靜默放棄。
//
// state.json 記錄它對應到 events.jsonl 的位元組數（events_bytes）。兩者不一致代表上次在
// 「追加事件後、寫 state 前」中斷，Update 會先重播補上；state.json 的版本與 engine.StateVersion
// 不符時同樣重播。events.jsonl 結尾若有當機留下的半行，重播時略過並截掉；中間行損毀則回傳錯誤，
// 不默默丟資料。事件的 At 保留 adapter 給的時區（每日補給依當地日期），只去掉單調時鐘。
//
// Load 與 History 不取鎖、不寫檔：state.json 以 rename 原子替換，讀到的一定是完整檔案；
// 版本不符時 Load 只在記憶體重播。
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

const (
	eventsFile = "events.jsonl"
	stateFile  = "state.json"
	lockFile   = "lock"
)

// DefaultLockTimeout 配合「hook 每次低於 100 ms」的目標。
const DefaultLockTimeout = 50 * time.Millisecond

var ErrBusy = errors.New("store: 另一個行程正在寫入")

type FileStore struct {
	dir         string
	pack        *content.Pack
	LockTimeout time.Duration
}

// snapshot 是 state.json 的實際內容：State 的欄位攤平，加上 events_bytes。
type snapshot struct {
	engine.State
	EventsBytes int64 `json:"events_bytes"`
}

// Open 建立（必要時）資料目錄並回傳 store；不取鎖、不讀檔。
func Open(dir string, pack *content.Pack) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileStore{dir: dir, pack: pack, LockTimeout: DefaultLockTimeout}, nil
}

func (s *FileStore) path(name string) string { return filepath.Join(s.dir, name) }

// Load 讀取 state.json；還沒有任何資料時回傳零值 State（V == 0）。
func (s *FileStore) Load() (engine.State, error) {
	snap, err := s.readSnapshot()
	if err != nil || !outdated(snap) {
		return snap.State, err
	}
	data, err := s.readEvents()
	if err != nil {
		return engine.State{}, err
	}
	snap, _, err = s.replayData(data, nil)
	return snap.State, err
}

// History 從 events.jsonl 重播，回傳所有冒險日誌（舊到新）。日誌不另外存檔，永遠由事件重算。
func (s *FileStore) History() ([]engine.LogLine, error) {
	data, err := s.readEvents()
	if err != nil {
		return nil, err
	}
	var logs []engine.LogLine
	_, _, err = s.replayData(data, func(r engine.Result) { logs = append(logs, r.Logs...) })
	return logs, err
}

func outdated(snap snapshot) bool {
	return snap.V != 0 && snap.V != engine.StateVersion
}

func (s *FileStore) Update(e event.Event) (engine.Result, error) {
	unlock, err := s.lock()
	if err != nil {
		return engine.Result{}, err
	}
	defer unlock()

	snap, err := s.readSnapshot()
	if err != nil {
		return engine.Result{}, err
	}
	size, err := s.eventsSize()
	if err != nil {
		return engine.Result{}, err
	}
	if snap.EventsBytes != size || outdated(snap) {
		if snap, err = s.replay(); err != nil {
			return engine.Result{}, err
		}
	}

	e.V = event.Version
	e.Seq = snap.LastSeq + 1
	e.At = e.At.Round(0) // 去掉單調時鐘，保留時區
	r := engine.Apply(snap.State, e, s.pack)

	n, err := s.appendEvent(e)
	if err != nil {
		return engine.Result{}, err
	}
	if err := s.writeSnapshot(snapshot{State: r.State, EventsBytes: snap.EventsBytes + n}); err != nil {
		return engine.Result{}, err
	}
	return r, nil
}

// Rebuild 從 events.jsonl 重播並覆寫 state.json。
func (s *FileStore) Rebuild() (engine.State, error) {
	unlock, err := s.lock()
	if err != nil {
		return engine.State{}, err
	}
	defer unlock()

	snap, err := s.replay()
	if err != nil {
		return engine.State{}, err
	}
	return snap.State, s.writeSnapshot(snap)
}

func (s *FileStore) lock() (unlock func(), err error) {
	fl := flock.New(s.path(lockFile))
	ctx, cancel := context.WithTimeout(context.Background(), s.LockTimeout)
	defer cancel()
	ok, err := fl.TryLockContext(ctx, 2*time.Millisecond)
	if !ok {
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { _ = fl.Unlock() }, nil
}

// replay 必須在鎖內呼叫：它可能截掉 events.jsonl 結尾的半行。
func (s *FileStore) replay() (snapshot, error) {
	data, err := s.readEvents()
	if err != nil {
		return snapshot{}, err
	}
	snap, torn, err := s.replayData(data, nil)
	if err != nil {
		return snapshot{}, err
	}
	if torn {
		if err := os.Truncate(s.path(eventsFile), snap.EventsBytes); err != nil {
			return snapshot{}, err
		}
	}
	return snap, nil
}

// replayData 依序套用每一行事件；visit 不為 nil 時收到每個事件的 Result。
// torn 表示結尾有當機留下的半行（已略過）。
func (s *FileStore) replayData(data []byte, visit func(engine.Result)) (snap snapshot, torn bool, err error) {
	var st engine.State
	good := 0
	for good < len(data) {
		nl := bytes.IndexByte(data[good:], '\n')
		if nl < 0 {
			break // 沒有換行的結尾：當機留下的半行
		}
		var e event.Event
		if err := json.Unmarshal(data[good:good+nl], &e); err != nil {
			if good+nl+1 == len(data) {
				break // 最後一行壞掉，同樣視為半行
			}
			return snapshot{}, false, fmt.Errorf("%s 在位元組 %d 損毀: %w", eventsFile, good, err)
		}
		r := engine.Apply(st, e, s.pack)
		if visit != nil {
			visit(r)
		}
		st = r.State
		good += nl + 1
	}
	return snapshot{State: st, EventsBytes: int64(good)}, good < len(data), nil
}

func (s *FileStore) readEvents() ([]byte, error) {
	data, err := os.ReadFile(s.path(eventsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (s *FileStore) eventsSize() (int64, error) {
	fi, err := os.Stat(s.path(eventsFile))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func (s *FileStore) appendEvent(e event.Event) (int64, error) {
	line, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(s.path(eventsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return 0, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return 0, err
	}
	return int64(len(line)), f.Close()
}

func (s *FileStore) readSnapshot() (snapshot, error) {
	data, err := os.ReadFile(s.path(stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return snapshot{}, nil
	}
	if err != nil {
		return snapshot{}, err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return snapshot{}, fmt.Errorf("%s: %w", stateFile, err)
	}
	return snap, nil
}

// writeSnapshot 先寫同目錄的暫存檔、fsync，再 rename 蓋過 state.json。
func (s *FileStore) writeSnapshot(snap snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(s.dir, stateFile+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // rename 成功後已不存在，失敗時清掉殘檔
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(stateFile))
}
