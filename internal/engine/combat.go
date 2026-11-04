package engine

import "github.com/s0912758806p/commit-dnd/content"

// PetAtk 是寵物的攻擊力：pet_atk_base + pet_atk_per_level × 等級 + 武器強度。
func PetAtk(s State, c *content.Pack) int {
	d := c.Balance.Dungeon
	return d.PetAtkBase + d.PetAtkPerLevel*s.Pet.Level + s.Dungeon.Weapon.Power
}

// PetDef 是寵物的防禦力：pet_def_per_level × 等級 + 防具強度。
func PetDef(s State, c *content.Pack) int {
	return c.Balance.Dungeon.PetDefPerLevel*s.Pet.Level + s.Dungeon.Armor.Power
}
