package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/event"
	"github.com/spf13/cobra"
)

// `dnd event <source> <type> [key=value...] [--count N]`：手動寫入事件，開發與驗收用；
// `dnd debug event` 是同一個指令。不是 hook，錯誤照常回報。
//
// 大部分 key=value 放進 Args；以下幾個會填進 Event 本身的欄位，才能模擬 adapter 產生的事件：
//
//   - repo=X、tool=X：Event.Repo、Event.Tool（例如 `claude tool_use tool=Edit` 會讓寵物攻擊）
//   - type 為 commit 時一定帶 CommitInfo：kind（預設 other）、added、deleted、hash。
//     沒給 hash 時自動產生不重複的 hash，否則引擎會當成同一筆 commit 不計分。
//
// --count N 一次寫入 N 筆（同一個時間、各自的 Seq 與 hash）。
func newEventCmd() *cobra.Command {
	var count int
	cmd := &cobra.Command{
		Use:   "event <source> <type> [key=value...]",
		Short: "手動寫入事件（開發用）",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if count < 1 {
				return fmt.Errorf("--count 至少要 1：%d", count)
			}
			at := clock.Now()
			events := make([]event.Event, count)
			for i := range events {
				e, err := buildEvent(args, at, i)
				if err != nil {
					return err
				}
				events[i] = e
			}

			s, _, err := openStore()
			if err != nil {
				return err
			}
			for _, e := range events {
				r, err := s.Update(e)
				if err != nil {
					return err
				}
				printf(cmd, "#%d %s/%s\n", r.State.LastSeq, e.Source, e.Type)
				for _, l := range r.Logs {
					printf(cmd, "  %s\n", l.Text)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&count, "count", 1, "一次寫入幾筆")
	return cmd
}

// buildEvent 把 `<source> <type> [key=value...]` 轉成事件；i 是 --count 的第幾筆，用來產生不重複的 commit hash。
func buildEvent(args []string, at time.Time, i int) (event.Event, error) {
	e := event.Event{At: at, Source: event.Source(args[0]), Type: args[1]}
	if !e.Source.Valid() {
		return e, fmt.Errorf("未知來源 %q（git|claude|codex|user|tick）", args[0])
	}
	if e.Type == "commit" {
		e.Commit = &event.CommitInfo{Kind: "other"}
	}
	for _, kv := range args[2:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return e, fmt.Errorf("參數要寫成 key=value：%q", kv)
		}
		switch {
		case k == "repo":
			e.Repo = v
		case k == "tool":
			e.Tool = v
		case e.Commit != nil && (k == "kind" || k == "hash"):
			if k == "kind" {
				e.Commit.Kind = v
			} else {
				e.Commit.Hash = v
			}
		case e.Commit != nil && (k == "added" || k == "deleted"):
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return e, fmt.Errorf("%s 要是非負整數：%q", k, v)
			}
			if k == "added" {
				e.Commit.Added = n
			} else {
				e.Commit.Deleted = n
			}
		default:
			if e.Args == nil {
				e.Args = map[string]string{}
			}
			e.Args[k] = v
		}
	}
	if e.Commit != nil && e.Commit.Hash == "" {
		e.Commit.Hash = fmt.Sprintf("debug-%x-%d", at.UnixNano(), i)
	}
	return e, nil
}
