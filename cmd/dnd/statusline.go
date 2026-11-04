package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/engine"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/install"
	"github.com/s0912758806p/commit-dnd/internal/view"
	"github.com/spf13/cobra"
)

// chainTimeout 是串接原本 statusLine 時等它的上限；超過就只顯示寵物。
const chainTimeout = time.Second

// `dnd statusline`：Claude Code 底部的一行。不取鎖、不寫檔、永遠 exit 0。
// 安裝前若已有別人的 statusLine（存在 install.OriginalStatuslinePath），先用同一份 stdin 執行它，
// 再把寵物資訊接在同一行後面。
func newStatuslineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "statusline",
		Short: "給 Claude Code 的一行狀態列",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			var in []byte
			if !interactive(cmd) { // 手動在終端機執行時沒有 stdin，不要卡住
				in, _ = io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			}
			var parts []string
			if orig := chainedStatusline(in); orig != "" {
				parts = append(parts, orig)
			}
			parts = append(parts, petLine())
			printf(cmd, "%s", strings.Join(parts, "  "))
		},
	}
}

func petLine() string {
	s, pack, err := openStore()
	if err != nil {
		return "dnd：讀取失敗（dnd doctor）"
	}
	st, err := s.Load()
	if err != nil {
		return "dnd：讀取失敗（dnd doctor）"
	}
	now := clock.Now()
	st = engine.Apply(st, event.Event{At: now, Source: event.SourceTick, Type: "tick"}, pack).State
	return view.Statusline(st, pack, now)
}

// chainedStatusline 執行被 dnd 取代前的 statusLine，回傳它輸出的第一行；沒有或失敗時回傳空字串。
func chainedStatusline(stdin []byte) string {
	dir, err := home.Dir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(install.OriginalStatuslinePath(dir))
	if err != nil {
		return ""
	}
	var orig struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(raw, &orig) != nil || orig.Command == "" || install.IsDndCommand(orig.Command) {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), chainTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", orig.Command)
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", orig.Command)
	}
	c.Stdin = bytes.NewReader(stdin)
	out, err := c.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimRight(line, "\r")
}
