package sample

import "testing"

func TestParseMeshShape(t *testing.T) {
	for _, c := range []struct {
		shape         string
		width, height int
	}{
		{"5x5", 5, 5},
		{"7X7", 7, 7},
		{"8x4", 8, 4},
	} {
		w, h, err := parseMeshShape(c.shape)
		if err != nil || w != c.width || h != c.height {
			t.Errorf("parseMeshShape(%q) = %d, %d, %v; want %d, %d",
				c.shape, w, h, err, c.width, c.height)
		}
	}

	for _, shape := range []string{"", "7", "7x", "x7", "0x5", "-1x5", "axb", "7x7x7"} {
		if _, _, err := parseMeshShape(shape); err == nil {
			t.Errorf("parseMeshShape(%q) must fail", shape)
		}
	}
}
