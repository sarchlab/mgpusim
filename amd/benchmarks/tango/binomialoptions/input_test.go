package binomialoptions

import (
	"math"
	"testing"
)

func TestArchivedOptionInputRounding(t *testing.T) {
	seed := uint32(42)
	expected := map[[2]int]uint32{
		{4, 2}: 0x40039ed0,
		{5, 3}: 0x3f5d3da5,
		{6, 3}: 0x3f52d837,
	}
	ranges := [4][2]float32{{5, 200}, {1, 300}, {0.25, 10}, {0.1, 1}}
	for i := 0; i <= 6; i++ {
		for j, bounds := range ranges {
			bits := math.Float32bits(randRange(&seed, bounds[0], bounds[1]))
			if want, ok := expected[[2]int{i, j}]; ok && bits != want {
				t.Errorf("option %d field %d = %#x, want %#x", i, j, bits, want)
			}
		}
	}
}
