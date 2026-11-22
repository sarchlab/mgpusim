package cutcp

import (
	"slices"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
)

func TestCutCPBinPadding(t *testing.T) {
	tests := []struct {
		name    string
		spacing float32
		padding int
		binSide int
	}{
		{name: "aligned", spacing: 0.5, padding: 3, binSide: 9},
		{name: "unaligned", spacing: 1, padding: 4, binSide: 14},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Benchmark{State: &State{
				Arch: arch.CDNA3, GridSide: 24, NumAtoms: 64,
				GridSpacing: tc.spacing, Cutoff: 12,
			}}

			padding := b.configure()
			if padding != tc.padding || b.BinSide != tc.binSide {
				t.Fatalf("got padding %d and bin side %d, want %d and %d",
					padding, b.BinSide, tc.padding, tc.binSide)
			}
			wantOrigin := ((tc.padding*tc.binSide+tc.padding)*tc.binSide + tc.padding) * binDepth
			if b.BinOrigin != wantOrigin {
				t.Fatalf("bin origin = %d, want %d", b.BinOrigin, wantOrigin)
			}
			if !slices.Contains(b.Neighbors, BinOffset{X: -int32(tc.padding)}) {
				t.Fatalf("missing outer neighbor at x=%d", -tc.padding)
			}

			for i, atom := range b.binAtoms(padding) {
				if atom.Charge == 0 {
					continue
				}

				x := int(atom.X/binLength) + tc.padding
				y := int(atom.Y/binLength) + tc.padding
				z := int(atom.Z/binLength) + tc.padding
				wantBin := (z*tc.binSide+y)*tc.binSide + x
				if i/binDepth != wantBin {
					t.Fatalf("atom %v was put in bin %d, want %d", atom, i/binDepth, wantBin)
				}
			}
			if len(b.Atoms) != b.NumAtoms {
				t.Fatalf("binned %d atoms, want %d", len(b.Atoms), b.NumAtoms)
			}
		})
	}
}
