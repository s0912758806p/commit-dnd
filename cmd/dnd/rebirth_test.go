package main

import (
	"strings"
	"testing"
)

// 迭代 6 的驗收流程（CLI 版）：用除錯 XP 快轉到 Lv 30、取得終職，轉生兩次。
func TestRebirthFlow(t *testing.T) {
	t.Setenv("DND_HOME", t.TempDir())
	t.Setenv("DND_DEBUG", "1")
	// hatch 會讀目前資料夾的 git log 建立 commit 組成；在空資料夾執行，職業才不受所在 repo 的歷史影響
	// （CI 的淺層 clone 只有一筆 PR 合併 commit，會被算成 other 而判定為冒險者）。
	t.Chdir(t.TempDir())
	run := func(args ...string) string {
		t.Helper()
		out, err := dnd(t, args...)
		if err != nil {
			t.Fatalf("dnd %v: %v\n%s", args, err, out)
		}
		return out
	}
	run("hatch", "--species", "dwarf", "--name", "小矮")
	if _, err := dnd(t, "rebirth", "--yes"); err == nil || !strings.Contains(err.Error(), "取得終職") {
		t.Fatalf("沒有終職不能轉生: %v", err)
	}
	for life := 1; life <= 2; life++ {
		run("debug", "xp", "5000")
		if out := run("class", "choose", "marksman"); !strings.Contains(out, "選擇了特化") { // 沒有 commit 組成時判定為清單第一個職業（遊俠）
			t.Fatalf("第 %d 輪選特化失敗:\n%s", life, out)
		}
		out := run("debug", "xp", "50000")
		if !strings.Contains(out, "破曉射手") {
			t.Fatalf("第 %d 輪應取得終職:\n%s", life, out)
		}
		if _, err := dnd(t, "rebirth"); err == nil || !strings.Contains(err.Error(), "--yes") {
			t.Fatalf("非互動又沒有 --yes 應拒絕: %v", err)
		}
		out = run("rebirth", "--yes")
		if !strings.Contains(out, "重置：") || !strings.Contains(out, "轉生了") {
			t.Fatalf("第 %d 次轉生:\n%s", life, out)
		}
	}
	status := run("status")
	if !strings.Contains(status, "轉生  2 次（基礎能力 +20%、經驗 +40%）") || !strings.Contains(status, "等級  Lv 1") {
		t.Fatalf("status:\n%s", status)
	}
	class := run("class")
	if !strings.Contains(class, "破曉射手 Lv 2：合體技倍率 +100%") {
		t.Fatalf("同一終職再次取得要強化一級:\n%s", class)
	}
}
