package block

type RootedDirt struct{ base }

func (RootedDirt) EncodeBlock() (string, map[string]any) { return "minecraft:dirt_with_roots", nil }
