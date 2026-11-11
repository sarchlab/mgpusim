package main

import (
	"fmt"
	"sort"
	"strings"
)

// A dimension is one parameter of a test configuration and the values it
// takes.
type dimension struct {
	name   string
	values []string
}

// A config assigns one value to every dimension, indexed like the dimensions.
type config []string

// value returns the value of the named dimension.
func (c config) value(dims []dimension, name string) string {
	for i, d := range dims {
		if d.name == name {
			return c[i]
		}
	}

	panic(fmt.Sprintf("acceptance: unknown dimension %q", name))
}

func (c config) String(dims []dimension) string {
	parts := make([]string, len(dims))
	for i, d := range dims {
		parts[i] = d.name + "=" + c[i]
	}

	return strings.Join(parts, " ")
}

// pairwise returns configurations that cover every pair of values of every two
// dimensions that some valid configuration covers (a 2-way covering array).
// valid rejects configurations that cannot run.
//
// It enumerates the valid configurations and greedily picks the one that
// covers the most uncovered pairs, so it is exact and deterministic but meant
// for small spaces (thousands of configurations). Larger spaces need an
// algorithm such as IPOG (sarchlab/mgpusim#343).
func pairwise(dims []dimension, valid func(config) bool) []config {
	all := enumerate(dims, valid)
	uncovered := map[string]bool{}

	for _, c := range all {
		for _, p := range pairsOf(c) {
			uncovered[p] = true
		}
	}

	picked := []config{}
	used := make([]bool, len(all))

	for len(uncovered) > 0 {
		best, bestGain := -1, 0

		for i, c := range all {
			if used[i] {
				continue
			}

			gain := 0
			for _, p := range pairsOf(c) {
				if uncovered[p] {
					gain++
				}
			}

			if gain > bestGain {
				best, bestGain = i, gain
			}
		}

		used[best] = true
		picked = append(picked, all[best])

		for _, p := range pairsOf(all[best]) {
			delete(uncovered, p)
		}
	}

	return picked
}

// enumerate returns every valid configuration, in a fixed order.
func enumerate(dims []dimension, valid func(config) bool) []config {
	out := []config{}
	cur := make(config, len(dims))

	var walk func(i int)
	walk = func(i int) {
		if i == len(dims) {
			if valid(cur) {
				out = append(out, append(config(nil), cur...))
			}

			return
		}

		for _, v := range dims[i].values {
			cur[i] = v
			walk(i + 1)
		}
	}

	walk(0)

	return out
}

// pairsOf returns the keys of the value pairs a configuration covers.
func pairsOf(c config) []string {
	keys := []string{}

	for i := range c {
		for j := i + 1; j < len(c); j++ {
			keys = append(keys, fmt.Sprintf("%d=%s,%d=%s", i, c[i], j, c[j]))
		}
	}

	sort.Strings(keys)

	return keys
}
