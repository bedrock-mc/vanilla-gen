package block

type MangroveRoots struct{ base }

func (MangroveRoots) EncodeBlock() (string, map[string]any) { return "minecraft:mangrove_roots", nil }
