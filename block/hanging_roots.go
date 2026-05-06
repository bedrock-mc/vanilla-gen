package block

type HangingRoots struct{ base }

func (HangingRoots) EncodeBlock() (string, map[string]any) { return "minecraft:hanging_roots", nil }
