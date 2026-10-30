// Package git 把 post-commit 當下的 git 輸出轉成 commit 事件。
//
// 輸入是 Command 這條 git 指令的輸出：HEAD 最新一筆 reflog 的
// 「hash ␟ reflog 說明 ␟ commit 標題 ␞」加上 --numstat。
//
//   - 只有 reflog 說明以 "commit: " 或 "commit (initial): " 開頭才計分；
//     amend、cherry-pick、rebase、merge 都會跑 post-commit，但不重複給 XP。
//   - 類型取自 Conventional Commits 標題（feat(scope)!: ...），不符合格式時為 "other"。
//   - 改動行數略過二進位檔與常見 lockfile。
//   - 標題本身不寫進事件，只留類型。
package git

import (
	"bytes"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/event"
)

// Command 是 hook 要執行的 git 參數；輸出交給 Adapter.Parse。
var Command = []string{"log", "-g", "-1", "--format=%H%x1f%gs%x1f%s%x1e", "--numstat", "--no-renames", "HEAD"}

type Adapter struct {
	Repo string // 只存資料夾名稱
}

var conventional = regexp.MustCompile(`^([A-Za-z]+)(\([^)]*\))?!?:`)

var lockfiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true,
	"go.sum": true, "Cargo.lock": true, "poetry.lock": true, "uv.lock": true, "Pipfile.lock": true,
	"composer.lock": true, "Gemfile.lock": true, "flake.lock": true,
}

func (a Adapter) Parse(raw []byte, now time.Time) ([]event.Event, error) {
	head, body, ok := bytes.Cut(raw, []byte{0x1e})
	if !ok {
		return nil, nil
	}
	fields := strings.Split(strings.TrimSpace(string(head)), "\x1f")
	if len(fields) != 3 || fields[0] == "" {
		return nil, nil
	}
	hash, reflog, subject := fields[0], fields[1], fields[2]
	if !strings.HasPrefix(reflog, "commit: ") && !strings.HasPrefix(reflog, "commit (initial): ") {
		return nil, nil
	}

	added, deleted := numstat(body)
	return []event.Event{{
		At:     now,
		Source: event.SourceGit,
		Type:   "commit",
		Repo:   a.Repo,
		Commit: &event.CommitInfo{Kind: Kind(subject), Added: added, Deleted: deleted, Hash: hash},
	}}, nil
}

// Kind 回傳 Conventional Commits 的類型（小寫）；不符合格式時為 "other"。
func Kind(subject string) string {
	m := conventional.FindStringSubmatch(subject)
	if m == nil {
		return "other"
	}
	return strings.ToLower(m[1])
}

// maxLines 以上的單檔行數視為壞資料，避免相加溢位。
const maxLines = 1 << 30

func numstat(body []byte) (added, deleted int) {
	for line := range strings.SplitSeq(string(body), "\n") {
		cols := strings.SplitN(line, "\t", 3)
		if len(cols) != 3 || lockfiles[path.Base(cols[2])] {
			continue
		}
		a, errA := strconv.Atoi(cols[0])
		d, errD := strconv.Atoi(cols[1])
		if errA != nil || errD != nil || a < 0 || d < 0 || a > maxLines || d > maxLines { // 二進位檔是 "-"
			continue
		}
		added += a
		deleted += d
	}
	return added, deleted
}
