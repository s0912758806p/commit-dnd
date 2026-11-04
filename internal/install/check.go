package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// 給 `dnd doctor` 用的唯讀檢查：只讀檔，不改任何東西。

// GitHookStatus 回報 hooksDir/post-commit 是否有 dnd 區塊，以及區塊裡呼叫的 dnd 路徑。
func GitHookStatus(hooksDir string) (dndPath string, installed bool, err error) {
	b, err := os.ReadFile(filepath.Join(hooksDir, hookName))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	start, stop, ok := findBlock(string(b))
	if !ok {
		return "", false, nil
	}
	for line := range strings.SplitSeq(string(b)[start:stop], "\n") {
		if strings.Contains(line, " hook git") {
			return unquoteFirstWord(line), true, nil
		}
	}
	return "", true, nil
}

// ClaudeStatus 是 Claude Code settings.json 裡 dnd 相關設定的現況。
type ClaudeStatus struct {
	PostToolUse, Stop, Statusline bool
	DndPaths                      []string // 這些設定呼叫的 dnd 路徑（去重）
}

// ClaudeSettingsStatus 讀取 settings.json；檔案不存在時回傳零值與 nil。
func ClaudeSettingsStatus(settingsPath string) (ClaudeStatus, error) {
	var st ClaudeStatus
	b, err := os.ReadFile(settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	var s struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return st, err
	}
	seen := map[string]bool{}
	note := func(cmd string) bool {
		if !IsDndCommand(cmd) {
			return false
		}
		if p := unquoteFirstWord(cmd); !seen[p] {
			seen[p] = true
			st.DndPaths = append(st.DndPaths, p)
		}
		return true
	}
	for event, groups := range s.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if note(h.Command) {
					switch event {
					case "PostToolUse":
						st.PostToolUse = true
					case "Stop":
						st.Stop = true
					}
				}
			}
		}
	}
	st.Statusline = note(s.StatusLine.Command)
	return st, nil
}

// unquoteFirstWord 取出指令的第一個詞（可能被單引號或雙引號包住）。
func unquoteFirstWord(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd != "" && (cmd[0] == '\'' || cmd[0] == '"') {
		q := cmd[0]
		var b strings.Builder
		for i := 1; i < len(cmd); i++ {
			switch {
			case q == '\'' && strings.HasPrefix(cmd[i:], `'\''`):
				b.WriteByte('\'')
				i += 3
			case cmd[i] == q:
				return b.String()
			default:
				b.WriteByte(cmd[i])
			}
		}
		return b.String()
	}
	first, _, _ := strings.Cut(cmd, " ")
	return first
}
