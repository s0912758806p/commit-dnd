package install

// Claude Code 整合（迭代 2）：編輯使用者的 settings.json。
//
//   - 設定檔位置：--settings 優先，否則 $CLAUDE_CONFIG_DIR/settings.json，否則 ~/.claude/settings.json；
//     不存在時建立（含上層目錄），不是合法 JSON 時報錯、不動檔案。
//   - install：hooks.PostToolUse（matcher "*"）與 hooks.Stop 各加一組 async 的 HookCommand；
//     事件裡已有 dnd 的 hook 時只更新 command。statusLine 換成 StatuslineCommand，
//     原本若是別人的，原樣存到 OriginalStatuslinePath，並保留 padding 等其他欄位。
//   - uninstall：移除所有事件裡 dnd 的 hook，只刪掉被我們清空的 group／事件陣列／hooks 物件；
//     statusLine 是 dnd 的時候，從 OriginalStatuslinePath 還原（並刪檔），沒有就刪掉 key。
//   - 編輯保留原文（見 claude_json.go），沒動到的 key 順序、值、縮排都不變。
//   - 寫入流程：印 diff → 確認（--yes 跳過；非終端機又沒 --yes 時拒絕）→ 備份成
//     <path>.dnd-backup-YYYYMMDD-HHMMSS（同權限）→ 暫存檔＋rename 原子寫入，保留原檔權限。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeSettingsPath 決定要編輯的 settings.json。
func ClaudeSettingsPath(flag string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Abs(filepath.Join(d, "settings.json"))
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".claude", "settings.json"), nil
}

// ClaudePlan 是算好但還沒寫入的修改。
type ClaudePlan struct {
	Path     string
	Old, New []byte
	Exists   bool
	Mode     os.FileMode
	Notes    []string // 除了 settings.json 以外的動作，寫入前印給使用者看

	origPath       string // OriginalStatuslinePath
	saveOriginal   []byte // install：要存下來的原 statusLine
	removeOriginal bool   // uninstall：還原後刪掉
}

// Changed 回報 settings.json 是否需要改。
func (p *ClaudePlan) Changed() bool { return !bytes.Equal(p.Old, p.New) }

// Diff 回傳 settings.json 的 unified diff。
func (p *ClaudePlan) Diff() string { return unifiedDiff(p.Path, p.Old, p.New) }

type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async"`
	Timeout int    `json:"timeout"`
}

type claudeHookGroup struct {
	Matcher string       `json:"matcher,omitempty"`
	Hooks   []claudeHook `json:"hooks"`
}

type claudeStatusline struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// claudeEvents 是 dnd 要掛的事件與 matcher（Stop 沒有 matcher）。
var claudeEvents = []struct{ name, matcher string }{{"PostToolUse", "*"}, {"Stop", ""}}

// PlanClaudeInstall 算出安裝後的 settings.json。
func PlanClaudeInstall(path, dndPath, dndHome string) (*ClaudePlan, error) {
	p, root, err := loadClaudeSettings(path, dndHome)
	if err != nil {
		return nil, err
	}

	hooks := root.get("hooks")
	if hooks == nil {
		hooks = newJSON(struct{}{})
		root.set("hooks", hooks)
	} else if hooks.kind != '{' {
		return nil, fmt.Errorf("%s 的 hooks 不是物件，請手動處理", path)
	}
	hookCmd := HookCommand(dndPath)
	for _, ev := range claudeEvents {
		groups := hooks.get(ev.name)
		if groups == nil {
			groups = newJSON([]any{})
			hooks.set(ev.name, groups)
		} else if groups.kind != '[' {
			return nil, fmt.Errorf("%s 的 hooks.%s 不是陣列，請手動處理", path, ev.name)
		}
		found := false
		for _, h := range dndHooks(groups) {
			found = true
			if c, _ := h.get("command").str(); c != hookCmd {
				h.set("command", newJSON(hookCmd))
			}
		}
		if !found {
			groups.push(newJSON(claudeHookGroup{
				Matcher: ev.matcher,
				Hooks:   []claudeHook{{Type: "command", Command: hookCmd, Async: true, Timeout: 10}},
			}))
		}
	}

	slCmd := StatuslineCommand(dndPath)
	switch sl := root.get("statusLine"); {
	case sl == nil:
		root.set("statusLine", newJSON(claudeStatusline{"command", slCmd}))
	case isDndStatusline(sl):
		if c, _ := sl.get("command").str(); c != slCmd {
			sl.set("command", newJSON(slCmd))
		}
	default:
		var orig bytes.Buffer
		if err := json.Indent(&orig, sl.raw, "", "  "); err != nil {
			return nil, err
		}
		p.saveOriginal = append(orig.Bytes(), '\n')
		p.Notes = append(p.Notes, "原本的 statusLine 會存到 "+p.origPath+"，dnd statusline 會先執行它再接上寵物資訊")
		if sl.kind != '{' {
			root.set("statusLine", newJSON(claudeStatusline{"command", slCmd}))
			break
		}
		if t, _ := sl.get("type").str(); t != "command" {
			sl.set("type", newJSON("command"))
		}
		sl.set("command", newJSON(slCmd))
	}

	return p.finish(root), nil
}

// PlanClaudeUninstall 算出移除 dnd 後的 settings.json。
func PlanClaudeUninstall(path, dndHome string) (*ClaudePlan, error) {
	p, root, err := loadClaudeSettings(path, dndHome)
	if err != nil || !p.Exists {
		return p, err
	}

	if hooks := root.get("hooks"); hooks != nil && hooks.kind == '{' {
		emptied := false
		for i := 0; i < len(hooks.items); {
			if removeDndHooks(hooks.items[i]) && len(hooks.items[i].items) == 0 {
				hooks.removeAt(i)
				emptied = true
				continue
			}
			i++
		}
		if emptied && len(hooks.items) == 0 {
			root.del("hooks")
		}
	}

	if sl := root.get("statusLine"); sl != nil && isDndStatusline(sl) {
		orig, err := os.ReadFile(p.origPath)
		switch {
		case errors.Is(err, os.ErrNotExist):
			root.del("statusLine")
		case err != nil:
			return nil, err
		default:
			n, err := parseJSON(orig)
			if err != nil {
				return nil, fmt.Errorf("%s %w，請手動處理", p.origPath, err)
			}
			n.markAll()
			root.set("statusLine", n)
			p.removeOriginal = true
			p.Notes = append(p.Notes, "statusLine 會從 "+p.origPath+" 還原，並刪除該檔")
		}
	}

	return p.finish(root), nil
}

func loadClaudeSettings(path, dndHome string) (*ClaudePlan, *jnode, error) {
	p := &ClaudePlan{Path: path, Mode: 0o600, origPath: OriginalStatuslinePath(dndHome)}
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		root, _ := parseJSON([]byte("{}"))
		return p, root, nil
	case err != nil:
		return nil, nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	p.Old, p.Exists, p.Mode = old, true, fi.Mode().Perm()
	root, err := parseJSON(old)
	if err != nil {
		return nil, nil, fmt.Errorf("%s %w，沒有修改", path, err)
	}
	if root.kind != '{' {
		return nil, nil, fmt.Errorf("%s 最外層不是物件，沒有修改", path)
	}
	return p, root, nil
}

// finish 排版新內容；只有真的改了才重新排版，保留檔案原本的頭尾空白（通常是結尾換行）。
func (p *ClaudePlan) finish(root *jnode) *ClaudePlan {
	if !root.changed() {
		p.New = p.Old
		return p
	}
	if !p.Exists {
		p.New = append(root.render("  "), '\n')
		return p
	}
	body := bytes.TrimSpace(p.Old)
	lead := p.Old[:bytes.Index(p.Old, body)]
	trail := p.Old[len(lead)+len(body):]
	p.New = append(append(append([]byte{}, lead...), root.render(detectIndent(p.Old))...), trail...)
	return p
}

func isDndStatusline(sl *jnode) bool {
	if sl.kind != '{' {
		return false
	}
	c, ok := sl.get("command").str()
	return ok && IsDndCommand(c)
}

// dndHooks 回傳事件陣列裡所有 dnd 的 hook 物件。
func dndHooks(groups *jnode) []*jnode {
	var out []*jnode
	for _, g := range groups.items {
		if g.kind != '{' {
			continue
		}
		hs := g.get("hooks")
		if hs == nil || hs.kind != '[' {
			continue
		}
		for _, h := range hs.items {
			if c, ok := h.get("command").str(); h.kind == '{' && ok && IsDndCommand(c) {
				out = append(out, h)
			}
		}
	}
	return out
}

// removeDndHooks 從事件陣列移除 dnd 的 hook，被清空的 group 一併移除；回報是否有移除。
func removeDndHooks(groups *jnode) bool {
	if groups.kind != '[' {
		return false
	}
	removed := false
	for i := 0; i < len(groups.items); {
		g := groups.items[i]
		var hs *jnode
		if g.kind == '{' {
			hs = g.get("hooks")
		}
		if hs == nil || hs.kind != '[' {
			i++
			continue
		}
		cut := false
		for j := 0; j < len(hs.items); {
			if c, ok := hs.items[j].get("command").str(); hs.items[j].kind == '{' && ok && IsDndCommand(c) {
				hs.removeAt(j)
				cut = true
				continue
			}
			j++
		}
		if cut && len(hs.items) == 0 {
			groups.removeAt(i)
			removed = true
			continue
		}
		removed = removed || cut
		i++
	}
	return removed
}

// Confirm 是寫入前確認需要的輸入輸出。
type Confirm struct {
	In          io.Reader
	Out         io.Writer
	Interactive bool // stdin 是終端機
	Yes         bool // --yes
	Now         time.Time
}

// Apply 印出 diff 與附帶動作、確認後備份並寫入；使用者拒絕時回傳 false。
// 呼叫前應先用 Changed 判斷是否需要寫入。
func (p *ClaudePlan) Apply(c Confirm) (bool, error) {
	fmt.Fprint(c.Out, p.Diff())
	for _, n := range p.Notes {
		fmt.Fprintln(c.Out, "※ "+n)
	}
	if !c.Yes {
		if !c.Interactive {
			return false, fmt.Errorf("stdin 不是終端機，確認內容後請加 --yes 寫入 %s", p.Path)
		}
		fmt.Fprintf(c.Out, "要寫入 %s 嗎？[y/N] ", p.Path)
		ans, _ := bufio.NewReader(c.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			fmt.Fprintln(c.Out, "已取消，沒有修改")
			return false, nil
		}
	}

	// 確認期間檔案被別人改了就放棄，免得蓋掉
	cur, err := os.ReadFile(p.Path)
	if p.Exists && (err != nil || !bytes.Equal(cur, p.Old)) || !p.Exists && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("%s 在確認期間被修改了，請重新執行", p.Path)
	}
	if p.Exists {
		backup, err := writeBackup(p.Path, p.Old, p.Mode, c.Now)
		if err != nil {
			return false, fmt.Errorf("備份失敗，沒有修改：%w", err)
		}
		fmt.Fprintln(c.Out, "已備份到 "+backup)
	}
	if p.saveOriginal != nil {
		if err := os.MkdirAll(filepath.Dir(p.origPath), 0o755); err != nil {
			return false, err
		}
		if err := writeAtomic(p.origPath, p.saveOriginal, 0o644); err != nil {
			return false, err
		}
	}
	if err := writeAtomic(p.Path, p.New, p.Mode); err != nil {
		return false, err
	}
	if p.removeOriginal {
		if err := os.Remove(p.origPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, err
		}
	}
	return true, nil
}

// writeBackup 寫 <path>.dnd-backup-YYYYMMDD-HHMMSS；同一秒內重複時加序號，不覆蓋舊備份。
func writeBackup(path string, data []byte, mode os.FileMode, now time.Time) (string, error) {
	base := path + ".dnd-backup-" + now.Format("20060102-150405")
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Chmod(name, mode) // 不受 umask 影響
		}
		return name, err
	}
}

// writeAtomic 寫暫存檔再 rename；path 是 symlink（dotfiles 管理）時寫到它指向的檔案。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".dnd-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // rename 成功後已不存在
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
