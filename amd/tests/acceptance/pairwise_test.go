package main

import "testing"

func TestPairwiseCoversEveryValidPair(t *testing.T) {
	dims := []dimension{
		{"a", []string{"a1", "a2", "a3"}},
		{"b", []string{"b1", "b2"}},
		{"c", []string{"c1", "c2", "c3"}},
		{"d", []string{"d1", "d2"}},
	}
	// a3 never runs with b2.
	valid := func(c config) bool {
		return !(c.value(dims, "a") == "a3" && c.value(dims, "b") == "b2")
	}

	got := pairwise(dims, valid)

	want := map[string]bool{}
	for _, c := range enumerate(dims, valid) {
		for _, p := range pairsOf(c) {
			want[p] = true
		}
	}

	covered := map[string]bool{}
	for _, c := range got {
		if !valid(c) {
			t.Fatalf("invalid configuration picked: %s", c.String(dims))
		}

		for _, p := range pairsOf(c) {
			covered[p] = true
		}
	}

	for p := range want {
		if !covered[p] {
			t.Errorf("pair %s not covered", p)
		}
	}

	if len(got) >= len(enumerate(dims, valid)) {
		t.Errorf("pairwise picked %d configurations, no fewer than the full product", len(got))
	}
}

func TestPageMigrationSuite(t *testing.T) {
	n := 0
	for _, b := range pageMigrationBenchmarks() {
		for _, c := range b.cases {
			n++

			if c.pageMigration == "" || !c.unifiedMemory || len(c.gpus) < 2 {
				t.Errorf("%s: page migration case without unified memory on 2+ GPUs", b.executable)
			}

			if c.gpuType == "mi300x" && len(c.gpus) > 2 {
				t.Errorf("%s: more than two MI300X GPUs", b.executable)
			}
		}
	}

	if n == 0 {
		t.Fatal("no page migration cases")
	}
}
