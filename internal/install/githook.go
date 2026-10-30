// Package install 負責把 dnd 接進外部工具（迭代 1：git post-commit hook）。
//
// git hook 以標記區塊寫入，安裝與移除都只動區塊本身，不碰使用者原有的內容：
//
//		# >>> dnd >>>
//		'/abs/path/dnd' hook git >/dev/null 2>&1 || true
//		# <<< dnd <<<
//
//	  - hooks 目錄用 `git rev-parse --git-path hooks` 取得，所以會尊重 core.hooksPath（husky 等）。
//	  - 已有 post-commit 時，區塊插在 shebang 的下一行，避免原腳本中途 exit 讓 dnd 不被執行；
//	    原腳本不是 shell（例如 node、python）時拒絕安裝，請使用者手動串接。
//	  - 寫入 dnd 的絕對路徑，因為 GUI git 工具的 PATH 常常不含 ~/go/bin。
//	  - 移除後若只剩 shebang，整個檔案刪掉。
package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	blockBegin = "# >>> dnd >>>"
	blockEnd   = "# <<< dnd <<<"
	hookName   = "post-commit"
	shebang    = "#!/bin/sh"
)

// GitHooksDir 回傳 repo 實際使用的 hooks 目錄（絕對路徑）。
func GitHooksDir(repo string) (string, error) {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "--git-path", "hooks").Output()
	if err != nil {
		return "", fmt.Errorf("%s 不是 git repo：%w", repo, err)
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repo, dir)
	}
	return filepath.Abs(dir)
}

// Block 回傳要寫進 post-commit 的區塊。
func Block(dndPath string) string {
	quoted := "'" + strings.ReplaceAll(dndPath, "'", `'\''`) + "'"
	return blockBegin + "\n" + quoted + " hook git >/dev/null 2>&1 || true\n" + blockEnd + "\n"
}

// InstallGitHook 在 hooksDir/post-commit 寫入或更新 dnd 區塊，回傳給使用者看的結果說明。
func InstallGitHook(hooksDir, dndPath string) (string, error) {
	path := filepath.Join(hooksDir, hookName)
	block := Block(dndPath)
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(hooksDir, 0o755); err != nil {
			return "", err
		}
		return "已建立 " + path, os.WriteFile(path, []byte(shebang+"\n"+block), 0o755)
	case err != nil:
		return "", err
	}

	text := string(old)
	var updated, msg string
	if start, stop, ok := findBlock(text); ok {
		updated = text[:start] + block + text[stop:]
		msg = "已更新 " + path
	} else {
		first, rest, _ := strings.Cut(text, "\n")
		if !isShell(first) {
			return "", fmt.Errorf("%s 不是 shell 腳本，請手動在裡面呼叫：\n%s", path, block)
		}
		updated = first + "\n" + block + rest
		msg = "已加入既有的 " + path
	}
	if updated == text {
		msg = "已經安裝過 " + path
	} else if err := os.WriteFile(path, []byte(updated), 0o755); err != nil {
		return "", err
	}
	return msg, ensureExecutable(path)
}

// UninstallGitHook 移除 dnd 區塊；檔案只剩 shebang 時整個刪掉。
func UninstallGitHook(hooksDir string) (string, error) {
	path := filepath.Join(hooksDir, hookName)
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "沒有安裝（找不到 " + path + "）", nil
	}
	if err != nil {
		return "", err
	}
	text := string(old)
	start, stop, ok := findBlock(text)
	if !ok {
		return "沒有安裝（" + path + " 裡沒有 dnd 區塊）", nil
	}
	rest := text[:start] + text[stop:]
	if t := strings.TrimSpace(rest); t == "" || t == shebang {
		return "已刪除 " + path, os.Remove(path)
	}
	return "已從 " + path + " 移除 dnd 區塊", os.WriteFile(path, []byte(rest), 0o755)
}

// findBlock 回傳區塊的起訖位置（含結尾換行）。
func findBlock(text string) (start, stop int, ok bool) {
	start = strings.Index(text, blockBegin)
	if start < 0 {
		return 0, 0, false
	}
	end := strings.Index(text[start:], blockEnd)
	if end < 0 {
		return 0, 0, false
	}
	stop = start + end + len(blockEnd)
	if stop < len(text) && text[stop] == '\n' {
		stop++
	}
	return start, stop, true
}

// isShell 判斷 shebang 是否為 sh 家族（sh、bash、zsh…），含 "#!/usr/bin/env bash" 寫法。
func isShell(first string) bool {
	if !strings.HasPrefix(first, "#!") {
		return false
	}
	f := strings.Fields(strings.TrimPrefix(first, "#!"))
	if len(f) == 0 {
		return false
	}
	prog := f[0]
	if strings.HasSuffix(prog, "/env") && len(f) >= 2 {
		prog = f[1]
	}
	return strings.HasSuffix(prog, "sh")
}

func ensureExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.Chmod(path, fi.Mode()|0o111)
}
