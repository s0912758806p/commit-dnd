package content

import (
	"bytes"
	"strings"
	"testing"
)

// 職業、特化與進化階段的名稱不可和種族名稱重複或互相包含（避免「騎士」「人族騎士」之類的混淆）。
func TestGrowthNamesDistinct(t *testing.T) {
	p, _ := Default()
	names := map[string]string{}
	add := func(name, where string) {
		if prev, ok := names[name]; ok {
			t.Errorf("%s 的名稱「%s」與 %s 重複", where, name, prev)
		}
		names[name] = where
	}
	for _, c := range p.Growth.Classes {
		add(c.Name, "職業 "+c.ID)
		for _, s := range c.Specs {
			add(s.Name, "特化 "+s.ID)
		}
		for _, sp := range p.Species {
			if strings.Contains(c.Name, sp.Name) || strings.Contains(sp.Name, c.Name) {
				t.Errorf("職業 %s 和種族 %s 名稱相近", c.Name, sp.Name)
			}
		}
	}
	for _, sp := range p.Species {
		add(sp.Name, "種族 "+sp.ID)
		for i, n := range p.Growth.StageNames[sp.ID] {
			add(n, sp.ID+" 第 "+string(rune('1'+i))+" 階段")
		}
	}
	// 每個 commit 組成的分類都要有職業能拿到
	for _, k := range MixKinds {
		found := false
		for _, c := range p.Growth.Classes {
			for _, ck := range c.Kinds {
				found = found || ck == k
			}
		}
		if !found {
			t.Errorf("commit 類型 %s 沒有對應的職業", k)
		}
	}
}

func TestGrowthLookups(t *testing.T) {
	p, _ := Default()
	if c, ok := p.ClassByID("knight"); !ok || c.Name != "騎士" {
		t.Fatalf("%+v", c)
	}
	if _, ok := p.SpecByID("knight", "marksman"); ok {
		t.Fatal("別的職業的特化不應找到")
	}
	if s, ok := p.SpecByID("knight", "paladin"); !ok || s.Name != "聖騎士" {
		t.Fatalf("%+v", s)
	}
	for level, want := range map[int]int{1: 1, 9: 1, 10: 2, 19: 2, 20: 3, 99: 3} {
		if got := p.StageFor(level); got != want {
			t.Fatalf("StageFor(%d) = %d", level, got)
		}
	}
	if got := p.StageName("knight", 2); got != "勇者" { // 舊 id 也查得到
		t.Fatalf("%q", got)
	}
	if p.StageName("human", 0) != "" || p.StageName("human", 4) != "" || p.StageName("nobody", 1) != "" {
		t.Fatal("超出範圍應為空字串")
	}
	if MixIndex("feat") != 0 || MixIndex("chore") != MixN-1 || MixIndex("") != MixN-1 {
		t.Fatal("MixIndex")
	}
}

func TestParseRejectsGrowth(t *testing.T) {
	ok := Embedded()
	with := func(old, new string) Files {
		f := ok
		if !bytes.Contains(ok.Classes, []byte(old)) {
			t.Fatalf("classes.yaml 裡找不到 %q", old)
		}
		f.Classes = bytes.Replace(ok.Classes, []byte(old), []byte(new), 1)
		return f
	}
	cases := map[string]Files{
		"未知欄位":        with("judge_level: 5", "judge_level: 5\ntypo: 1"),
		"特化等級不高於判定等級": with("spec_level: 15", "spec_level: 5"),
		"不存在的分類":      with("kinds: [feat]", "kinds: [feature]"),
		"分類重複":        with("kinds: [fix]", "kinds: [feat]"),
		"職業 id 重複":    with("id: cleric", "id: ranger"),
		"特化 id 與職業重複": with("id: marksman", "id: knight"),
		"權重為 0":       with("weight_pct: 150", "weight_pct: 0"),
		"階段不從 1 開始":   with("{level: 1,  atk: 0", "{level: 2,  atk: 0"),
		"階段等級不遞增":     with("{level: 20,", "{level: 10,"),
		"缺少種族的階段名稱":   with("  demon:     [小鬼, 惡魔, 魔王]\n", ""),
		"階段名稱數量不符":    with("[小鬼, 惡魔, 魔王]", "[小鬼, 惡魔]"),
		"掉落機率超過 100":  with("drop_pct: 50", "drop_pct: 101"),
		"日誌缺句":        with(`judge: "{name} 依 commit 組成轉職為{class}（{style}）！"`, `judge: ""`),
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(f); err == nil {
				t.Fatal("應該回傳錯誤")
			}
		})
	}
}
