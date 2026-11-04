package engine

// 除錯事件：`dnd debug xp`／`dnd debug floor` 寫進 events.jsonl 的事件，和一般事件一樣會被重播，
// 所以 Rebuild 後結果相同。CLI 只在 DND_DEBUG=1 時提供這兩個指令，但引擎不看環境變數，照常套用。
//
//   - debug_xp：Args["xp"] 是正整數，加進 XP 後照正常流程連續升級（HP 回滿），不吃補給倍率與飢餓減半。
//   - debug_floor：Args["floor"] 是非負整數，直接設定樓層、清掉眼前的怪物、這層進度歸零，
//     並更新最深樓層。倒地狀態、傳送點與背包不變；下一個 AI 回合在新樓層遇到第一隻怪物（0 是入口，會往下探到 B1）。
//
// 參數不合法或還沒孵蛋時只留一句日誌、狀態不變。日誌文字寫死在這裡（開頭「【除錯】」），不放進 content。

import (
	"strconv"

	"github.com/s0912758806p/commit-dnd/content"
	"github.com/s0912758806p/commit-dnd/internal/event"
)

func debugEvent(r *Result, e event.Event, c *content.Pack) {
	if !r.State.Pet.Hatched() {
		r.log(e.At, c, c.Logs.NoPet, nil)
		return
	}
	switch e.Type {
	case "debug_xp":
		n, err := strconv.Atoi(e.Args["xp"])
		if err != nil || n <= 0 {
			r.log(e.At, c, "【除錯】XP 要是正整數：{arg}", map[string]string{"arg": e.Args["xp"]})
			return
		}
		r.State.Pet.XP += n
		r.log(e.At, c, "【除錯】{name} 獲得 {xp} XP", map[string]string{"xp": strconv.Itoa(n)})
		levelUp(r, e, c)
	case "debug_floor":
		n, err := strconv.Atoi(e.Args["floor"])
		if err != nil || n < 0 {
			r.log(e.At, c, "【除錯】樓層要是非負整數：{arg}", map[string]string{"arg": e.Args["floor"]})
			return
		}
		d := &r.State.Dungeon
		d.Floor, d.Cleared, d.Monster = n, 0, Monster{}
		d.Deepest = max(d.Deepest, n)
		where := "入口"
		if n > 0 {
			where = "B" + strconv.Itoa(n)
		}
		r.log(e.At, c, "【除錯】{name} 被傳到{where}", map[string]string{"where": where})
	}
}
