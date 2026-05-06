package block

type Spawner struct{ base }

func (Spawner) EncodeBlock() (string, map[string]any) {
	return "minecraft:mob_spawner", nil
}
