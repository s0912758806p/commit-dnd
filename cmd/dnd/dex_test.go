package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDexAndParty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	for _, args := range [][]string{{"dex"}, {"party"}} {
		if out, err := dnd(t, args...); err != nil || out != "還沒有寵物。用 dnd hatch 孵一隻吧。\n" {
			t.Fatalf("%v 沒有寵物時應提示孵蛋: %v %q", args, err, out)
		}
	}
	if _, err := dnd(t, "hatch", "--species", "elf", "--name", "小精"); err != nil {
		t.Fatal(err)
	}
	// 第一個 AI 回合下探 B1，圖鑑記下遇到的怪物
	if _, err := dnd(t, "event", "claude", "turn"); err != nil {
		t.Fatal(err)
	}
	out, err := dnd(t, "dex")
	if err != nil || !strings.HasPrefix(out, "圖鑑  收服 0/10 種一般怪物，遇過 1/13 種\n") || !strings.Contains(out, "遇過 1 次，還沒收服") {
		t.Fatalf("dex: %v\n%s", err, out)
	}
	out, err = dnd(t, "party")
	if err != nil || !strings.HasPrefix(out, "隊伍  0/3，還沒有隊員") || !strings.Contains(out, "合體技  就緒") {
		t.Fatalf("剛有 AI 回合，合體技應就緒: %v\n%s", err, out)
	}
	// 能不能加入由引擎判斷：沒收服過時回一句日誌、指令成功
	if out, err := dnd(t, "party", "add", "null_slime"); err != nil || out != "無法調整隊伍：還沒收服過 Null 史萊姆。\n" {
		t.Fatalf("party add: %v %q", err, out)
	}
	if out, err := dnd(t, "party", "remove", "ghost"); err != nil || out != "無法調整隊伍：ghost 不在隊伍裡。\n" {
		t.Fatalf("party remove: %v %q", err, out)
	}
}

// party 的參數個數由 CLI 檢查，錯誤時不寫事件。
func TestPartyArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DND_HOME", home)
	for _, args := range [][]string{{"party", "add"}, {"party", "remove"}, {"party", "add", "a", "b"}, {"party", "swap", "a"}, {"dex", "x"}} {
		if _, err := dnd(t, args...); err == nil {
			t.Fatalf("%v 應報錯", args)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("參數錯誤時不應寫入事件: %v", err)
	}
}
