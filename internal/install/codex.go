package install

// Codex CLI 整合（迭代 4）：編輯使用者的 config.toml，只動最上層的 notify 一行。
//
//   - 設定檔位置：--config 優先，否則 $CODEX_HOME/config.toml，否則 ~/.codex/config.toml；
//     不存在時建立（含上層目錄），不是合法 TOML 時報錯、不動檔案。
//   - Codex 在每個 agent 回合結束時執行 notify 陣列（程式＋參數），JSON payload 附在最後一個參數。
//   - install：沒有 notify（或是空陣列）→ 設成 CodexNotify（`["<dnd>", "hook", "codex"]`）；
//     已經是 dnd 的 → 只更新路徑；是別人的 → 原本的陣列與原文存到 OriginalNotifyPath，
//     再換成 dnd，由 `dnd hook codex` 處理完自己的事件後，用同樣的參數執行原本的 notify。
//   - uninstall：notify 是 dnd 的時候，從 OriginalNotifyPath 還原原文（並刪檔），沒有就刪掉那一行。
//   - 編輯保留原文（見 codex_toml.go），改完重新解析，確認 notify 以外的資料完全相同才寫入。
//   - 寫入流程與 Claude Code 相同（ClaudePlan.Apply）：印 diff → 確認 → 備份 → 原子寫入。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ConfigPlan 是「算好但還沒寫入的設定檔修改」；Codex 與 Claude Code 共用同一套 diff／確認／備份／寫入。
type ConfigPlan = ClaudePlan

// CodexConfigPath 決定要編輯的 config.toml。
func CodexConfigPath(flag string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return filepath.Abs(filepath.Join(d, "config.toml"))
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".codex", "config.toml"), nil
}

// CodexNotify 回傳 installer 寫進 config.toml 的 notify 陣列。
func CodexNotify(dndPath string) []string { return []string{dndPath, "hook", "codex"} }

// IsDndNotify 回報 notify 陣列是不是 dnd 裝的（不論 dnd 裝在哪個路徑）。
func IsDndNotify(argv []string) bool {
	if len(argv) != 3 || argv[1] != "hook" || argv[2] != "codex" {
		return false
	}
	base := filepath.Base(argv[0])
	return base == "dnd" || base == "dnd.exe"
}

// OriginalNotifyPath 是被 dnd 取代前的 notify 存放位置。
func OriginalNotifyPath(dndHome string) string {
	return filepath.Join(dndHome, "codex", "original-notify.json")
}

// originalNotify 是 OriginalNotifyPath 的內容：Notify 給 `dnd hook codex` 執行，TOML 是原文，uninstall 時原樣還原。
type originalNotify struct {
	Notify []string `json:"notify"`
	TOML   string   `json:"toml,omitempty"`
}

// ReadOriginalNotify 讀取安裝前原本的 notify；沒有串接時回傳 nil。
func ReadOriginalNotify(dndHome string) ([]string, error) {
	o, err := readOriginalNotify(OriginalNotifyPath(dndHome))
	if err != nil || o == nil {
		return nil, err
	}
	return o.Notify, nil
}

func readOriginalNotify(path string) (*originalNotify, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o originalNotify
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("%s 不是合法的 JSON：%w", path, err)
	}
	return &o, nil
}

// codexConfig 是讀進來的 config.toml。
type codexConfig struct {
	plan    *ConfigPlan
	data    map[string]any
	entries []tomlEntry
	notify  *tomlEntry // 最上層的 notify；沒有時為 nil
	argv    []string   // 目前的 notify
	nl      string     // 檔案用的換行
}

// PlanCodexInstall 算出安裝後的 config.toml。
func PlanCodexInstall(path, dndPath, dndHome string) (*ConfigPlan, error) {
	c, err := loadCodexConfig(path, dndHome)
	if err != nil {
		return nil, err
	}
	want := CodexNotify(dndPath)
	switch {
	case c.notify == nil:
		c.insert(want)
	case IsDndNotify(c.argv):
		if slices.Equal(c.argv, want) {
			c.plan.New = c.plan.Old
			return c.plan, nil
		}
		c.replace(tomlArray(want))
	case len(c.argv) > 0:
		var orig bytes.Buffer
		enc := json.NewEncoder(&orig)
		enc.SetEscapeHTML(false) // 指令裡常有 > 與 &，保持可讀
		enc.SetIndent("", "  ")
		if err := enc.Encode(originalNotify{c.argv, string(c.plan.Old[c.notify.valStart:c.notify.valEnd])}); err != nil {
			return nil, err
		}
		c.plan.saveOriginal = orig.Bytes()
		c.plan.Notes = append(c.plan.Notes, fmt.Sprintf("原本的 notify（%s）會存到 %s，dnd hook codex 處理完自己的事件後，會用同樣的參數執行它",
			strings.Join(c.argv, " "), c.plan.origPath))
		c.replace(tomlArray(want))
	default: // notify = []
		c.replace(tomlArray(want))
	}
	return c.verified(want)
}

// PlanCodexUninstall 算出移除 dnd 後的 config.toml。
func PlanCodexUninstall(path, dndHome string) (*ConfigPlan, error) {
	c, err := loadCodexConfig(path, dndHome)
	if err != nil {
		return nil, err
	}
	if c.notify == nil || !IsDndNotify(c.argv) {
		c.plan.New = c.plan.Old
		return c.plan, nil
	}
	orig, err := readOriginalNotify(c.plan.origPath)
	if err != nil {
		return nil, fmt.Errorf("%w，請手動處理", err)
	}
	if orig == nil || len(orig.Notify) == 0 || IsDndNotify(orig.Notify) {
		c.remove()
		return c.verified(nil)
	}
	c.plan.removeOriginal = true
	c.plan.Notes = append(c.plan.Notes, "notify 會從 "+c.plan.origPath+" 還原，並刪除該檔")
	if orig.TOML != "" { // 先試著還原原文；原文解析出來不一樣時改用陣列
		c.replace(orig.TOML)
		if p, err := c.verified(orig.Notify); err == nil {
			return p, nil
		}
	}
	c.replace(tomlArray(orig.Notify))
	return c.verified(orig.Notify)
}

func loadCodexConfig(path, dndHome string) (*codexConfig, error) {
	c := &codexConfig{plan: &ConfigPlan{Path: path, Mode: 0o600, origPath: OriginalNotifyPath(dndHome)}, nl: "\n"}
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.data = map[string]any{}
		return c, nil
	case err != nil:
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	c.plan.Old, c.plan.Exists, c.plan.Mode = old, true, fi.Mode().Perm()
	if bytes.Contains(old, []byte("\r\n")) {
		c.nl = "\r\n"
	}
	if c.data, err = decodeTOML(old); err != nil {
		return nil, fmt.Errorf("%s 不是合法的 TOML（%w），沒有修改", path, err)
	}
	argv, has, err := notifyArgv(c.data)
	if err != nil {
		return nil, fmt.Errorf("%s 的 %w，請手動處理", path, err)
	}
	if c.entries, _, err = scanTopLevel(old); err != nil {
		return nil, fmt.Errorf("讀不懂 %s（%w），沒有修改", path, err)
	}
	for i := range c.entries {
		if isNotifyKey(c.entries[i].key) {
			c.notify = &c.entries[i]
		}
	}
	if has && c.notify == nil {
		return nil, fmt.Errorf("%s 裡找不到 notify 那一行，請手動處理", path)
	}
	c.argv = argv
	return c, nil
}

// insert 在最後一個最上層的 key 後面加一行 notify；沒有最上層的 key 時加在檔案開頭。
// 不額外加空行，uninstall 刪掉這一行就會跟原檔一模一樣。
func (c *codexConfig) insert(want []string) {
	src := c.plan.Old
	line := "notify = " + tomlArray(want)
	var pos int
	var text string
	if n := len(c.entries); n > 0 {
		pos = c.entries[n-1].end
		if pos == len(src) && !bytes.HasSuffix(src, []byte("\n")) {
			text = c.nl + line // 原檔結尾沒有換行，維持沒有
		} else {
			text = line + c.nl
		}
	} else {
		pos = bomLen(src)
		text = line + c.nl
	}
	c.plan.New = splice(src, pos, pos, text)
}

// replace 把 notify 的值換成 value（TOML 原文）。
func (c *codexConfig) replace(value string) {
	c.plan.New = splice(c.plan.Old, c.notify.valStart, c.notify.valEnd, value)
}

// remove 刪掉 notify 那一行；在沒有結尾換行的檔案最後時，連同前面的換行（insert 加的）。
func (c *codexConfig) remove() {
	src, e := c.plan.Old, c.notify
	start, end := e.start, e.end
	if end == len(src) && !bytes.HasSuffix(src, []byte("\n")) && bytes.HasSuffix(src[:start], []byte(c.nl)) {
		start -= len(c.nl)
	}
	c.plan.New = splice(src, start, end, "")
}

// verified 確認改好的內容只有 notify 不同；不通過時不寫入。
func (c *codexConfig) verified(want []string) (*ConfigPlan, error) {
	if err := verifyCodexEdit(c.plan.New, c.data, want); err != nil {
		return nil, fmt.Errorf("改寫 %s 後的檢查沒有通過（%v），沒有修改，請手動編輯 notify", c.plan.Path, err)
	}
	return c.plan, nil
}

func splice(src []byte, start, end int, text string) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(text))
	out = append(out, src[:start]...)
	out = append(out, text...)
	return append(out, src[end:]...)
}
