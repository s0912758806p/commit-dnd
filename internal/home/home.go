// Package home 決定資料目錄：DND_HOME 環境變數優先，否則為使用者家目錄下的 .dnd。
// 開發測試一律設 DND_HOME=./tmp/dnd，避免弄壞正式遊玩的寵物。
package home

import (
	"os"
	"path/filepath"
)

const EnvVar = "DND_HOME"

func Dir() (string, error) {
	if d := os.Getenv(EnvVar); d != "" {
		return d, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".dnd"), nil
}
