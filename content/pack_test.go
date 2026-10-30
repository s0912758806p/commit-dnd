package content

import "testing"

func TestDefaultParses(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash == "" || p.Balance.Mood.Max == 0 {
		t.Fatalf("內容未載入: %+v", p)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"未知欄位":     "mood: {start: 1, max: 10, decay_every_minutes: 1, typo: 1}",
		"週期為 0":    "mood: {start: 1, max: 10, decay_every_minutes: 0}",
		"起始超過上限":   "mood: {start: 11, max: 10, decay_every_minutes: 1}",
		"壞掉的 YAML": "mood: [",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Fatal("應該回傳錯誤")
			}
		})
	}
}
