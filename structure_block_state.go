package vanilla

import (
	"fmt"
	"strconv"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
)

func blockStateFromWorldBlock(b world.Block) gen.BlockState {
	name, properties := b.EncodeBlock()
	state := gen.BlockState{Name: normalizeStructureName(name)}
	if len(properties) == 0 {
		return state
	}
	state.Properties = make(map[string]string, len(properties))
	for key, value := range properties {
		state.Properties[key] = structurePropertyString(value)
	}
	return state
}

func structurePropertyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return boolString(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
