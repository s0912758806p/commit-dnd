package git

// 讀歷史 commit（迭代 5）：dnd hatch 用來建立 commit 組成，dnd sim 用來重播整個 repo。
// 只讀 git log，不寫 repo；類型與行數的判斷和 post-commit 相同（Kind、numstat）。

import (
	"bytes"
	"slices"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

// LogArgs 是讀取歷史 commit 的 git 參數：不含 merge，每筆「␞hash␟作者時間␟標題」後接 --numstat。
// since 是 git 認得的時間（例如 RFC 3339），author 是 --author 的樣式；空字串表示不限。
func LogArgs(since, author string) []string {
	args := []string{"log", "--no-merges", "--format=%x1e%H%x1f%aI%x1f%s", "--numstat", "--no-renames"}
	if since != "" {
		args = append(args, "--since="+since)
	}
	if author != "" {
		args = append(args, "--author="+author)
	}
	return args
}

// LogCommit 是歷史上的一筆 commit；At 是作者時間（保留當時的時區）。
type LogCommit struct {
	At     time.Time
	Commit event.CommitInfo
}

// ParseLog 解析 LogArgs 的輸出，依作者時間由舊到新排序（同時間保持 git 的順序）。格式不對的紀錄略過。
func ParseLog(raw []byte) []LogCommit {
	var out []LogCommit
	for rec := range bytes.SplitSeq(raw, []byte{0x1e}) {
		head, body, _ := bytes.Cut(rec, []byte("\n"))
		fields := strings.Split(string(head), "\x1f")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, fields[1])
		if err != nil {
			continue
		}
		added, deleted := numstat(body)
		out = append(out, LogCommit{At: at, Commit: event.CommitInfo{Kind: Kind(fields[2]), Added: added, Deleted: deleted, Hash: fields[0]}})
	}
	slices.Reverse(out) // git log 是新到舊
	slices.SortStableFunc(out, func(a, b LogCommit) int { return a.At.Compare(b.At) })
	return out
}
