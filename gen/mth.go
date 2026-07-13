package gen

import "math"

// mthSin is Mth.SIN: the 65536-entry float32 sine table vanilla uses for all
// carver trigonometry. Table lookups round differently than math.Sin, so
// carved shapes only match when going through the table.
var mthSin = func() *[65536]float32 {
	var table [65536]float32
	for i := range table {
		table[i] = float32(math.Sin(float64(i) * math.Pi * 2.0 / 65536.0))
	}
	return &table
}()

func MthSin(value float32) float32 {
	return mthSin[uint16(int32(value*10430.378))]
}

func MthCos(value float32) float32 {
	return mthSin[uint16(int32(value*10430.378+16384.0))]
}
