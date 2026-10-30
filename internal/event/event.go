// Package event 定義 adapter 與引擎之間唯一的資料形狀。
//
// 所有外部條件（時間、commit 資訊、指令參數）都放進 Event，引擎不讀時鐘、不碰檔案，
// 因此 events.jsonl 可以完整重播。At 由 adapter 填入；Seq 與 V 由 store 在鎖內指派，
// adapter 不要自己設定。
package event

import "time"

// Version 是事件 JSON 格式的版本；格式有不相容變動時加一，重播時依此轉換舊事件。
const Version = 1

type Source string

const (
	SourceGit    Source = "git"
	SourceClaude Source = "claude"
	SourceCodex  Source = "codex"
	SourceUser   Source = "user"
	SourceTick   Source = "tick"
)

// Valid 回報 s 是否為已知來源。
func (s Source) Valid() bool {
	switch s {
	case SourceGit, SourceClaude, SourceCodex, SourceUser, SourceTick:
		return true
	}
	return false
}

// CommitInfo 描述一次 commit；Hash 之後也作為亂數種子的一部分。
type CommitInfo struct {
	Kind    string `json:"kind"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Hash    string `json:"hash"`
}

type Event struct {
	V      int               `json:"v"`
	Seq    int64             `json:"seq"` // store 指派，從 1 開始連續遞增；未落地的 tick 為 0
	At     time.Time         `json:"at"`  // adapter 填入；保留時區（每日補給依當地日期），store 只去掉單調時鐘
	Source Source            `json:"source"`
	Type   string            `json:"type"`           // "commit" | "tool_use" | "turn" | "pet" | "warp" | "rebirth" ...
	Repo   string            `json:"repo,omitempty"` // 只存資料夾名稱
	Commit *CommitInfo       `json:"commit,omitempty"`
	Tool   string            `json:"tool,omitempty"` // Claude 的 tool_name
	Args   map[string]string `json:"args,omitempty"` // 指令參數，例如 hatch 的物種、warp 的樓層
}
