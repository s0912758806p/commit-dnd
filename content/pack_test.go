package content

import (
	"bytes"
	"testing"
)

func TestDefaultParses(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash == "" || len(p.Species) != 8 || p.Cost(1) == 0 {
		t.Fatalf("內容未載入: hash=%q species=%d", p.Hash, len(p.Species))
	}
}

// 迭代 1 的驗收「第一次 commit 升到 Lv 2」依賴這條：最低的基本 XP × 補給倍率 ≥ Lv1→2 的成本。
func TestFirstCommitAlwaysReachesLevel2(t *testing.T) {
	p, _ := Default()
	lowest := p.Balance.Commit.Base["other"]
	for _, xp := range p.Balance.Commit.Base {
		lowest = min(lowest, xp)
	}
	if got := lowest * p.Balance.Supply.Multiplier; got < p.Cost(1) {
		t.Fatalf("最低 commit XP %d < Lv1→2 成本 %d", got, p.Cost(1))
	}
}

// docs/DESIGN-NOTES.md 寫死的數字（Lv29→30 為 1433、到 Lv30 共 18,795）綁住實算值。
func TestXPTableMatchesDesignNotes(t *testing.T) {
	p, _ := Default()
	total := 0
	for n := 1; n < 30; n++ {
		total += p.Cost(n)
	}
	if p.Cost(1) != 18 || p.Cost(29) != 1433 || total != 18795 {
		t.Fatalf("Cost(1)=%d Cost(29)=%d total=%d", p.Cost(1), p.Cost(29), total)
	}
}

func TestParseRejects(t *testing.T) {
	ok := Embedded()
	cases := map[string]Files{
		"balance 未知欄位": {Balance: append(bytes.Clone(ok.Balance), "\ntypo: 1\n"...), Species: ok.Species, Logs: ok.Logs},
		"balance 壞掉":   {Balance: []byte("xp: ["), Species: ok.Species, Logs: ok.Logs},
		"沒有 other":     {Balance: bytes.Replace(ok.Balance, []byte("other: 10"), []byte("misc: 10"), 1), Species: ok.Species, Logs: ok.Logs},
		"物種重複":         {Balance: ok.Balance, Species: []byte("- {id: a, name: A}\n- {id: a, name: B}\n"), Logs: ok.Logs},
		"舊 id 與 id 重複": {Balance: ok.Balance, Species: []byte("- {id: a, name: A}\n- {id: b, name: B, aliases: [a]}\n"), Logs: ok.Logs},
		"沒有物種":         {Balance: ok.Balance, Species: []byte("[]"), Logs: ok.Logs},
		"日誌沒有 other":   {Balance: ok.Balance, Species: ok.Species, Logs: []byte("commit: {feat: [x]}")},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(f); err == nil {
				t.Fatal("應該回傳錯誤")
			}
		})
	}
}

// 舊物種「人類騎士」（knight）併入人族：舊存檔的孵蛋事件要能對應到 human。
func TestSpeciesAliases(t *testing.T) {
	p, _ := Default()
	sp, ok := p.SpeciesByID("knight")
	if !ok || sp.ID != "human" || sp.Name != "人族" {
		t.Fatalf("knight → %+v %v", sp, ok)
	}
	if _, ok := p.SpeciesByID("slime"); ok {
		t.Fatal("slime 已移除")
	}
}
