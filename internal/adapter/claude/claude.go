// Package claude 把 Claude Code hook 的 stdin JSON 轉成事件。
//
//   - PostToolUse → tool_use 事件（Tool = tool_name）
//   - Stop → turn 事件（一個 AI 回合結束）
//   - 其他 hook、缺少必要欄位、不是 JSON 物件：不產生事件
//
// 只讀 hook_event_name、tool_name、cwd；tool_input／tool_response、session_id、
// transcript_path 都不寫進事件。Repo 取 cwd 的資料夾名稱。
package claude

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

type Adapter struct{}

type payload struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	Cwd           string `json:"cwd"`
}

func (Adapter) Parse(raw []byte, now time.Time) ([]event.Event, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	e := event.Event{At: now, Source: event.SourceClaude}
	if p.Cwd != "" {
		e.Repo = filepath.Base(p.Cwd)
	}
	switch p.HookEventName {
	case "PostToolUse":
		if p.ToolName == "" {
			return nil, nil
		}
		e.Type, e.Tool = "tool_use", p.ToolName
	case "Stop":
		e.Type = "turn"
	default:
		return nil, nil
	}
	return []event.Event{e}, nil
}
