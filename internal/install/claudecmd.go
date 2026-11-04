package install

import (
	"path/filepath"
	"strings"
)

// Claude Code 整合的共同約定：installer 寫進 settings.json 的指令，以及 `dnd statusline`
// 讀取的串接檔，都以這裡為準。
//
//   - PostToolUse 與 Stop 兩個 hook 都呼叫 HookCommand（`'<dnd>' hook claude`），
//     由 stdin 的 hook_event_name 分辨事件。
//   - statusLine 呼叫 StatuslineCommand（`'<dnd>' statusline`）。安裝前若已有別人的 statusLine，
//     installer 把原本的 statusLine 物件原樣存到 OriginalStatuslinePath；`dnd statusline`
//     執行時會先跑它，再把寵物資訊接在同一行後面；uninstall 時再從這個檔案還原。
//   - IsDndCommand 用來辨認哪些 hook／statusLine 是 dnd 裝的。

// HookCommand 回傳 PostToolUse 與 Stop hook 的指令。
func HookCommand(dndPath string) string { return shellQuote(dndPath) + " hook claude" }

// StatuslineCommand 回傳 statusLine 的指令。
func StatuslineCommand(dndPath string) string { return shellQuote(dndPath) + " statusline" }

// IsDndCommand 回報指令是不是 dnd 裝的 hook 或 statusLine（不論 dnd 裝在哪個路徑）。
func IsDndCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if !strings.HasSuffix(cmd, " hook claude") && !strings.HasSuffix(cmd, " statusline") {
		return false
	}
	bin := strings.Trim(strings.Fields(cmd)[0], `'"`)
	base := filepath.Base(bin)
	return base == "dnd" || base == "dnd.exe"
}

// OriginalStatuslinePath 是被 dnd 取代前的 statusLine（JSON 物件）存放位置。
func OriginalStatuslinePath(dndHome string) string {
	return filepath.Join(dndHome, "claude", "original-statusline.json")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
