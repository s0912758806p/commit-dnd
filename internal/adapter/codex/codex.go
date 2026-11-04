// Package codex 把 Codex CLI 的 notify payload 轉成事件。
//
// Codex 在 config.toml 設了 notify 時，每個 agent 回合結束會執行該指令，並把 JSON payload
// 附在最後一個參數（stdin 是空的）。目前只有一種 type：
//
//   - agent-turn-complete → 兩個事件：先 tool_use（Tool = "Codex"），再 turn。
//     notify 沒有個別的工具呼叫，所以一個 Codex 回合算寵物攻擊一次（"Codex" 列在
//     balance.yaml 的 dungeon.attack_tools）再結束回合，節奏跟 Claude Code 相近。
//   - 其他 type、缺少 type 或 cwd、不是 JSON 物件：不產生事件
//
// 只讀 type 與 cwd；input-messages、last-assistant-message、thread-id、turn-id 都不寫進事件。
// Repo 取 cwd 的資料夾名稱。
package codex

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

// Tool 是 Codex 回合在 tool_use 事件裡的工具名稱。
const Tool = "Codex"

type Adapter struct{}

type payload struct {
	Type string `json:"type"`
	Cwd  string `json:"cwd"`
}

func (Adapter) Parse(raw []byte, now time.Time) ([]event.Event, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.Type != "agent-turn-complete" || p.Cwd == "" {
		return nil, nil
	}
	e := event.Event{At: now, Source: event.SourceCodex, Repo: filepath.Base(p.Cwd)}
	attack, turn := e, e
	attack.Type, attack.Tool = "tool_use", Tool
	turn.Type = "turn"
	return []event.Event{attack, turn}, nil
}
