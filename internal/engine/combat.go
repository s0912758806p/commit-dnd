package engine

import "github.com/s0912758806p/commit-dnd/content"

// PetAtk 是寵物的攻擊力：pet_atk_base + pet_atk_per_level × 等級 + 武器強度 + 隊伍加成（PartyAtk）。
func PetAtk(s State, c *content.Pack) int {
	d := c.Balance.Dungeon
	return d.PetAtkBase + d.PetAtkPerLevel*s.Pet.Level + s.Dungeon.Weapon.Power + PartyAtk(s, c)
}

// PetDef 是寵物的防禦力：pet_def_per_level × 等級 + 防具強度。
func PetDef(s State, c *content.Pack) int {
	return c.Balance.Dungeon.PetDefPerLevel*s.Pet.Level + s.Dungeon.Armor.Power
}

// FloorCleared 回報目前這層是否已經打完：還在入口，或已打倒 encounters_per_floor 隻（打倒 Boss 也算）。
// 打完時下一個 AI 回合會往下探，否則在同一層遇到下一隻。
func FloorCleared(s State, c *content.Pack) bool {
	return s.Dungeon.Floor == 0 || s.Dungeon.Cleared >= c.Balance.Dungeon.EncountersPerFloor
}
