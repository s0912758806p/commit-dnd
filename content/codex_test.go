package content

import "testing"

// Codex 的 notify 沒有個別工具呼叫，adapter 把每個回合記成一次 Tool = "Codex" 的 tool_use。
func TestCodexIsAttackTool(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAttackTool("Codex") {
		t.Fatal("balance.yaml 的 dungeon.attack_tools 應包含 Codex")
	}
}
