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
	if _, err := Parse(ok); err != nil {
		t.Fatalf("正式內容應該通過: %v", err)
	}
	with := func(edit func(f *Files)) Files { f := ok; edit(&f); return f }
	cases := map[string]Files{
		"balance 未知欄位": with(func(f *Files) { f.Balance = append(bytes.Clone(ok.Balance), "\ntypo: 1\n"...) }),
		"balance 壞掉":   with(func(f *Files) { f.Balance = []byte("xp: [") }),
		"沒有 other":     with(func(f *Files) { f.Balance = bytes.Replace(ok.Balance, []byte("other: 10"), []byte("misc: 10"), 1) }),
		"物種重複":         with(func(f *Files) { f.Species = []byte("- {id: a, name: A}\n- {id: a, name: B}\n") }),
		"舊 id 與 id 重複": with(func(f *Files) { f.Species = []byte("- {id: a, name: A}\n- {id: b, name: B, aliases: [a]}\n") }),
		"沒有物種":         with(func(f *Files) { f.Species = []byte("[]") }),
		"日誌沒有 other":   with(func(f *Files) { f.Logs = []byte("commit: {feat: [x]}") }),
		"怪物屬性錯誤": with(func(f *Files) {
			f.Monsters = bytes.Replace(ok.Monsters, []byte("element: water"), []byte("element: light"), 1)
		}),
		"樓層引用不存在的怪物": with(func(f *Files) { f.Floors = bytes.Replace(ok.Floors, []byte("null_slime"), []byte("ghost"), 1) }),
		"一般層放了 Boss": with(func(f *Files) { f.Floors = bytes.Replace(ok.Floors, []byte("[null_slime"), []byte("[bug_king"), 1) }),
		"缺少某屬性的裝備":   with(func(f *Files) { f.Items = []byte("weapon: {fire: [a]}\narmor: {fire: [b]}") }),
		"Boss 間隔為 0": with(func(f *Files) {
			f.Balance = bytes.Replace(ok.Balance, []byte("boss_every: 10"), []byte("boss_every: 0"), 1)
		}),
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(f); err == nil {
				t.Fatal("應該回傳錯誤")
			}
		})
	}
}

func TestDungeonLookups(t *testing.T) {
	p, _ := Default()
	if len(p.Monsters) < 10 {
		t.Fatalf("至少要 10 種一般怪物以上，實際 %d", len(p.Monsters))
	}
	for floor, want := range map[int]string{1: "", 9: "", 10: "bug_king", 20: "merge_hydra", 30: "legacy_drake", 40: "bug_king", 15: ""} {
		if got := p.BossFor(floor); got != want {
			t.Errorf("BossFor(%d) = %q, want %q", floor, got, want)
		}
	}
	if got := p.TierMonsters(1); got[0] != "null_slime" {
		t.Errorf("TierMonsters(1) = %v", got)
	}
	if got := p.TierMonsters(999); got[0] != "spaghetti" {
		t.Errorf("最深處用最後一段: %v", got)
	}
	if !p.IsAttackTool("Edit") || p.IsAttackTool("Read") {
		t.Error("攻擊工具判斷錯誤")
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
