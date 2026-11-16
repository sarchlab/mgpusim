package kernels

import (
	"encoding/json"
	"testing"
)

func TestWGSelectionSurvivesSerialization(t *testing.T) {
	for _, selection := range []WGSelection{
		{},
		{Partitioned: true, First: 0, End: 0},
		{Partitioned: true, First: 3, End: 13},
		{Partitioned: true, First: 20, End: 30},
	} {
		data, err := json.Marshal(selection)
		if err != nil {
			t.Fatal(err)
		}
		var restored WGSelection
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		var count uint64
		// A 3 x 2 x 4 grid exercises all three coordinates and partial ranges.
		for z := uint64(0); z < 4; z++ {
			for y := uint64(0); y < 2; y++ {
				for x := uint64(0); x < 3; x++ {
					index := z*6 + y*3 + x
					want := !selection.Partitioned ||
						(index >= selection.First && index < selection.End)
					if restored.Includes(index) != want {
						t.Fatalf("selection %+v changed for (%d,%d,%d)", selection, x, y, z)
					}
					if want {
						count++
					}
				}
			}
		}
		if got := restored.Count(24); got != count {
			t.Fatalf("selection %+v: count %d, want %d", restored, got, count)
		}
	}
}
