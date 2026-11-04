package install

import (
	"errors"
	"fmt"
	"os"
)

// 給 `dnd doctor` 用的唯讀檢查（Codex）：只讀檔，不改任何東西。

// CodexStatus 是 Codex config.toml 裡 dnd 相關設定的現況。
type CodexStatus struct {
	Exists    bool     // config.toml 存在
	Installed bool     // notify 是 dnd 的
	DndPath   string   // notify 呼叫的 dnd 路徑
	Original  []string // 串接的原本 notify；沒有時為 nil
}

// CodexConfigStatus 讀取 config.toml 與串接檔；config.toml 不存在時回傳零值與 nil。
func CodexConfigStatus(configPath, dndHome string) (CodexStatus, error) {
	var st CodexStatus
	b, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.Exists = true
	data, err := decodeTOML(b)
	if err != nil {
		return st, fmt.Errorf("不是合法的 TOML：%w", err)
	}
	argv, _, err := notifyArgv(data)
	if err != nil {
		return st, err
	}
	if !IsDndNotify(argv) {
		return st, nil
	}
	st.Installed, st.DndPath = true, argv[0]
	st.Original, err = ReadOriginalNotify(dndHome)
	return st, err
}
