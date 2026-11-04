package content

import "fmt"

// MaxMonsters 是 monsters.yaml 最多能有幾種怪物（含 Boss）。
// 引擎的圖鑑用這個大小的固定陣列存在 State 裡（State 要保持可比較），所以載入時驗證不超過。
const MaxMonsters = 64

// Party 是收服、合體技與隊伍的數值（balance.yaml 的 party 區塊，迭代 4）。
type Party struct {
	// 收服機率 = capture_base_pct + capture_missing_hp_pct × 怪物已損失 HP 的比例。
	CaptureBasePct      int `yaml:"capture_base_pct"`
	CaptureMissingHPPct int `yaml:"capture_missing_hp_pct"`
	MemberAtkPct        int `yaml:"member_atk_pct"`
	ComboWindowMinutes  int `yaml:"combo_window_minutes"`
	ComboBasePct        int `yaml:"combo_base_pct"`
	ComboPerMemberPct   int `yaml:"combo_per_member_pct"`
}

// PartyLogs 是收服、合體技與隊伍的日誌模板（logs.yaml 的 party 區塊）。
type PartyLogs struct {
	Capture     string `yaml:"capture"`
	CaptureFail string `yaml:"capture_fail"`
	CaptureBoss string `yaml:"capture_boss"`
	Join        string `yaml:"join"`
	Full        string `yaml:"full"`
	Combo       string `yaml:"combo"`
	Remove      string `yaml:"remove"`
	Denied      string `yaml:"denied"`
}

func (p *Pack) validateParty() error {
	b := p.Balance.Party
	if b.CaptureBasePct < 0 || b.CaptureMissingHPPct < 0 || b.CaptureBasePct+b.CaptureMissingHPPct > 100 ||
		b.MemberAtkPct < 0 || b.ComboWindowMinutes < 1 || b.ComboBasePct < 100 || b.ComboPerMemberPct < 0 {
		return fmt.Errorf("balance.yaml: party 的數值超出範圍（收服機率合計不可超過 100）")
	}
	if len(p.Monsters) > MaxMonsters {
		return fmt.Errorf("monsters.yaml: 最多 %d 種怪物（圖鑑的容量），實際 %d", MaxMonsters, len(p.Monsters))
	}
	l := p.Logs.Party
	for _, s := range []string{l.Capture, l.CaptureFail, l.CaptureBoss, l.Join, l.Full, l.Combo, l.Remove, l.Denied} {
		if s == "" {
			return fmt.Errorf("logs.yaml: party 的每一句都要有內容")
		}
	}
	return nil
}
