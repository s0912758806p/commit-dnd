package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/s0912758806p/commit-dnd/internal/clock"
	"github.com/s0912758806p/commit-dnd/internal/home"
	"github.com/spf13/cobra"
)

// `dnd debug time [+8h|-1h|reset]`：調整 internal/clock 的時間偏移（累加），不帶參數時顯示目前偏移。
// 偏移影響之後產生的事件與 status 等顯示的「現在」，已寫入的事件不變。
// 關掉旗標解析，`-1h` 才不會被當成旗標。
func newDebugTimeCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "time [+8h|-1h|reset]",
		Short:              "快轉或倒轉時間（偏移存在 DND_HOME，只在 DND_DEBUG=1 時生效）",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("只接受一個參數：+8h、-1h、reset")
			}
			dir, err := home.Dir()
			if err != nil {
				return err
			}
			off, err := clock.Offset(dir)
			if len(args) == 1 {
				switch arg := args[0]; arg {
				case "-h", "--help":
					return cmd.Help()
				case "reset":
					off, err = 0, nil
				default:
					d, perr := time.ParseDuration(strings.TrimPrefix(arg, "+"))
					if perr != nil {
						return fmt.Errorf("看不懂 %q：用 Go 的時間格式，例如 +8h、-30m、+48h", arg)
					}
					if err != nil {
						return fmt.Errorf("%w → dnd debug time reset", err)
					}
					off += d
				}
				if err := clock.SetOffset(dir, off); err != nil {
					return err
				}
			} else if err != nil {
				return fmt.Errorf("%w → dnd debug time reset", err)
			}
			printf(cmd, "時間偏移 %s，現在是 %s\n", clock.Format(off), time.Now().Add(off).Format("2006-01-02 15:04"))
			return nil
		},
	}
}

// debugTime 是 doctor 的一項：DND_HOME 裡殘留偏移檔時提醒（正式使用時不生效，但容易忘記）。
func (d *doctor) debugTime() {
	dir, err := home.Dir()
	if err != nil {
		return
	}
	off, err := clock.Offset(dir)
	switch {
	case err != nil:
		d.warn("除錯時間偏移", "%v → DND_DEBUG=1 dnd debug time reset", err)
	case off == 0:
	case clock.Enabled():
		d.warn("除錯時間偏移", "%s 生效中（DND_DEBUG=1）→ dnd debug time reset", clock.Format(off))
	default:
		d.warn("除錯時間偏移", "殘留 %s 的偏移檔，未設 DND_DEBUG=1 所以不生效 → DND_DEBUG=1 dnd debug time reset", clock.Format(off))
	}
}
