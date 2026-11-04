package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/s0912758806p/commit-dnd/internal/install"
	"github.com/spf13/cobra"
)

// `dnd doctor`：檢查資料目錄、寵物、hook 錯誤紀錄、git hook、Claude Code 與 Codex 設定。只讀，不改任何東西。
// 有 ✗ 的項目時 exit 1；! 只是提醒。
func newDoctorCmd() *cobra.Command {
	var repo, settings, codexConfig string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "檢查 dnd 的安裝與資料是否正常",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, err := install.ClaudeSettingsPath(settings)
			if err != nil {
				return err
			}
			codexConfig, err := install.CodexConfigPath(codexConfig)
			if err != nil {
				return err
			}
			d := &doctor{cmd: cmd}
			d.binary()
			d.data()
			d.debugTime()
			d.gitHook(repo)
			d.claude(settings)
			d.codex(codexConfig)
			if d.failed > 0 {
				return fmt.Errorf("有 %d 項需要處理", d.failed)
			}
			printf(cmd, "\n全部正常。\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", ".", "要檢查 git hook 的 repo")
	cmd.Flags().StringVar(&settings, "settings", "", "Claude Code 的 settings.json（預設依 CLAUDE_CONFIG_DIR 或 ~/.claude）")
	cmd.Flags().StringVar(&codexConfig, "codex-config", "", "Codex 的 config.toml（預設依 CODEX_HOME 或 ~/.codex）")
	return cmd
}

type doctor struct {
	cmd    *cobra.Command
	failed int
}

func (d *doctor) ok(label, format string, a ...any)   { d.line("✓", label, format, a...) }
func (d *doctor) warn(label, format string, a ...any) { d.line("!", label, format, a...) }
func (d *doctor) fail(label, format string, a ...any) {
	d.failed++
	d.line("✗", label, format, a...)
}
func (d *doctor) line(mark, label, format string, a ...any) {
	printf(d.cmd, "%s %s：%s\n", mark, label, fmt.Sprintf(format, a...))
}

func (d *doctor) binary() {
	if p, err := selfPath(); err != nil {
		d.warn("執行檔", "%v", err)
	} else {
		d.ok("執行檔", "%s", p)
	}
}

func (d *doctor) data() {
	dir, err := home.Dir()
	if err != nil {
		d.fail("資料目錄", "%v", err)
		return
	}
	s, _, err := openStore()
	if err != nil {
		d.fail("資料目錄", "%s：%v", dir, err)
		return
	}
	st, err := s.Load()
	if err != nil {
		d.fail("資料目錄", "%s：%v → DND_DEBUG=1 dnd debug rebuild", dir, err)
		return
	}
	d.ok("資料目錄", "%s（%d 個事件）", dir, st.LastSeq)
	if st.Pet.Hatched() {
		d.ok("寵物", "%s Lv %d", st.Pet.Name, st.Pet.Level)
	} else {
		d.fail("寵物", "還沒有寵物 → dnd hatch")
	}

	b, err := os.ReadFile(filepath.Join(dir, "errors.log"))
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		d.ok("hook 錯誤紀錄", "沒有")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	d.warn("hook 錯誤紀錄", "共 %d 筆，最近一筆：%s（%s）", len(lines), lines[len(lines)-1], filepath.Join(dir, "errors.log"))
}

func (d *doctor) gitHook(repo string) {
	dir, err := install.GitHooksDir(repo)
	if err != nil {
		d.warn("git hook", "%s 不是 git repo，略過", repo)
		return
	}
	p, ok, err := install.GitHookStatus(dir)
	switch {
	case err != nil:
		d.fail("git hook", "%v", err)
	case !ok:
		d.fail("git hook", "這個 repo 沒有安裝 → dnd install git")
	case !executable(p):
		d.fail("git hook", "post-commit 指向的 %s 不存在或不能執行 → 重新 dnd install git", p)
	default:
		d.ok("git hook", "已安裝（%s）", p)
	}
}

func (d *doctor) claude(settings string) {
	st, err := install.ClaudeSettingsStatus(settings)
	if err != nil {
		d.fail("Claude Code", "讀不了 %s：%v", settings, err)
		return
	}
	var missing []string
	for _, c := range []struct {
		name string
		ok   bool
	}{{"PostToolUse hook", st.PostToolUse}, {"Stop hook", st.Stop}, {"statusLine", st.Statusline}} {
		if !c.ok {
			missing = append(missing, c.name)
		}
	}
	for _, p := range st.DndPaths {
		if !executable(p) {
			d.fail("Claude Code", "設定指向的 %s 不存在或不能執行 → 重新 dnd install claude", p)
		}
	}
	if len(missing) > 0 {
		d.fail("Claude Code", "%s 未安裝（%s）→ dnd install claude", strings.Join(missing, "、"), settings)
		return
	}
	d.ok("Claude Code", "hooks 與 statusLine 已安裝（%s）", settings)
	if dir, err := home.Dir(); err == nil {
		if _, err := os.Stat(install.OriginalStatuslinePath(dir)); err == nil {
			d.ok("statusLine 串接", "會先顯示安裝前原本的 statusLine")
		}
	}
}

func executable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}
