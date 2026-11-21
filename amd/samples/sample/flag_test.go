package sample

import (
	"slices"
	"testing"
)

func TestParseGPUList(t *testing.T) {
	for _, c := range []struct {
		list   string
		numAll int
		want   []int
	}{
		{"1", 0, []int{1}},
		{"1,2,3,4", 0, []int{1, 2, 3, 4}},
		{"1-4", 0, []int{1, 2, 3, 4}},
		{"1-4,5-7", 0, []int{1, 2, 3, 4, 5, 6, 7}},
		{"1,3-5,8", 0, []int{1, 3, 4, 5, 8}},
		{"5-7,1-2", 0, []int{5, 6, 7, 1, 2}},
		{" 2 , 4-5 ", 0, []int{2, 4, 5}},
		{"3-3", 0, []int{3}},
		{"all", 3, []int{1, 2, 3}},
		{"ALL", 2, []int{1, 2}},
		{"1-24", 24, nil},
	} {
		got, err := parseGPUList(c.list, c.numAll)
		want := c.want
		if want == nil {
			for i := 1; i <= 24; i++ {
				want = append(want, i)
			}
		}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("parseGPUList(%q, %d) = %v, %v; want %v", c.list, c.numAll, got, err, want)
		}
	}

	for _, c := range []struct {
		list   string
		numAll int
	}{
		{"", 0},
		{"a", 0},
		{"0", 0},
		{"-1", 0},
		{"5-3", 0},
		{"1-", 0},
		{"-4", 0},
		{"1--4", 0},
		{"1,,2", 0},
		{"1,2,", 0},
		{"1-4,3", 0},
		{"all", 0},
		{"1-25", 24},
		{"all,1", 24},
	} {
		if got, err := parseGPUList(c.list, c.numAll); err == nil {
			t.Errorf("parseGPUList(%q, %d) = %v; want an error", c.list, c.numAll, got)
		}
	}
}

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
