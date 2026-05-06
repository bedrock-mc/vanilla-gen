package block

type MossBlock struct{ base }

func (MossBlock) EncodeBlock() (string, map[string]any) { return "minecraft:moss_block", nil }
