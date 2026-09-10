package lbm

import (
	"fmt"
	"math"
	"testing"
)

type lbmWrite struct {
	sourceCell int
	sourceQ    int
}

// forEachLBMWrite enumerates the destination chosen by the per-link streaming
// rule used by both cpuCollideStream and lbm_collide_stream_kernel.
func forEachLBMWrite(nx, ny, nz int, visit func(source, sourceQ, dest, destQ int)) {
	numCells := nx * ny * nz
	for source := 0; source < numCells; source++ {
		z := source / (nx * ny)
		y := (source / nx) % ny
		x := source % nx

		for q := 0; q < Q; q++ {
			destX := x + hEx[q]
			destY := y + hEy[q]
			destZ := z + hEz[q]

			if destX >= 0 && destX < nx &&
				destY >= 0 && destY < ny &&
				destZ >= 0 && destZ < nz {
				dest := destZ*nx*ny + destY*nx + destX
				visit(source, q, dest, q)
			} else {
				visit(source, q, source, hOpp[q])
			}
		}
	}
}

func TestPerLinkStreamingHasExactlyOneWriterPerDestinationSlot(t *testing.T) {
	grids := [][3]int{
		{1, 1, 1},
		{2, 2, 2},
		{3, 3, 3},
		{2, 3, 4},
	}

	for _, grid := range grids {
		nx, ny, nz := grid[0], grid[1], grid[2]
		t.Run(fmt.Sprintf("%dx%dx%d", nx, ny, nz), func(t *testing.T) {
			numCells := nx * ny * nz
			writers := make([][]lbmWrite, Q*numCells)

			forEachLBMWrite(nx, ny, nz, func(source, sourceQ, dest, destQ int) {
				slot := destQ*numCells + dest
				writers[slot] = append(writers[slot], lbmWrite{
					sourceCell: source,
					sourceQ:    sourceQ,
				})
			})

			for slot, slotWriters := range writers {
				if len(slotWriters) != 1 {
					t.Fatalf(
						"destination q=%d cell=%d has %d writers: %+v",
						slot/numCells, slot%numCells, len(slotWriters), slotWriters,
					)
				}
			}
		})
	}
}

func TestCPUCollideStreamUsesPerLinkDestinations(t *testing.T) {
	const n = 3
	numCells := n * n * n

	// With omega=0, collision leaves every distribution unchanged. Distinct
	// source values therefore identify exactly which link wrote each slot.
	src := make([]float32, Q*numCells)
	for i := range src {
		src[i] = float32(i + 1)
	}

	notWritten := math.Float32frombits(0x7fc00001)
	dst := make([]float32, len(src))
	expected := make([]float32, len(src))
	for i := range dst {
		dst[i] = notWritten
		expected[i] = notWritten
	}

	forEachLBMWrite(n, n, n, func(source, sourceQ, dest, destQ int) {
		expected[destQ*numCells+dest] = src[sourceQ*numCells+source]
	})
	cpuCollideStream(src, dst, n, 0)

	for slot := range expected {
		if math.Float32bits(dst[slot]) != math.Float32bits(expected[slot]) {
			t.Fatalf(
				"destination q=%d cell=%d: expected writer value %g, got %g",
				slot/numCells, slot%numCells, expected[slot], dst[slot],
			)
		}
	}
}
