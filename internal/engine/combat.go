package engine

import "github.com/s0912758806p/commit-dnd/content"

// PetAtk 是寵物的攻擊力：(pet_atk_base + pet_atk_per_level × 等級) × 攻擊加成 + 武器強度。
// 加成是職業＋特化＋進化（見 class.go 的 Bonus）。
func PetAtk(s State, c *content.Pack) int {
	d := c.Balance.Dungeon
	return pct(d.PetAtkBase+d.PetAtkPerLevel*s.Pet.Level, 100+Bonus(s.Pet, c).Atk) + s.Dungeon.Weapon.Power
}

// PetDef 是寵物的防禦力：pet_def_per_level × 等級 × 防禦加成 + 防具強度。
func PetDef(s State, c *content.Pack) int {
	return pct(c.Balance.Dungeon.PetDefPerLevel*s.Pet.Level, 100+Bonus(s.Pet, c).Def) + s.Dungeon.Armor.Power
}

// FloorCleared 回報目前這層是否已經打完：還在入口，或已打倒 encounters_per_floor 隻（打倒 Boss 也算）。
// 打完時下一個 AI 回合會往下探，否則在同一層遇到下一隻。
func FloorCleared(s State, c *content.Pack) bool {
	return s.Dungeon.Floor == 0 || s.Dungeon.Cleared >= c.Balance.Dungeon.EncountersPerFloor
}
