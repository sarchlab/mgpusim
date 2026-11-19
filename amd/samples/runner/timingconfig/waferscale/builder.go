// Package waferscale configures a baseline wafer-scale GPU: GPU tiles on a 2D
// mesh with the host in the centre. The baseline is Model 1 of Akkalat: a 5x5
// mesh whose 24 non-host positions each hold an R9 Nano-based tile of 32 CUs
// and 4 GB of memory.
//
// The mesh is independent of the tile microarchitecture. The GPU type
// "waferscale" uses the default tile; "waferscale-<tile>" names one.
package waferscale

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
)

// GPUType is the GPU type that selects a wafer-scale GPU.
const GPUType = "waferscale"

// SwitchLatency is the inter-switch latency of the mesh in cycles, Akkalat's
// default.
const SwitchLatency = 20

// Mesh is the shape of the wafer. The host (driver, IOMMU and CPU memory)
// sits on the centre position; every other position holds a GPU tile.
type Mesh struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DefaultMesh is Akkalat's Model 1: 5x5 positions, 24 tiles.
var DefaultMesh = Mesh{Width: 5, Height: 5}

// MustBeValid panics if the mesh has no position for a GPU tile.
func (m Mesh) MustBeValid() {
	if m.Width < 1 || m.Height < 1 || m.Width*m.Height < 2 {
		panic(fmt.Sprintf("waferscale: a %dx%d mesh has no GPU tile", m.Width, m.Height))
	}
}

// NumTiles returns the number of GPU tiles, one per non-host position.
func (m Mesh) NumTiles() int {
	return m.Width*m.Height - 1
}

// HostLocation returns the mesh coordinate of the host.
func (m Mesh) HostLocation() [3]int {
	return [3]int{m.Width / 2, m.Height / 2, 0}
}

// TileLocation returns the mesh coordinate of the GPU tile with the given
// 1-based index. Tiles fill the mesh row by row and skip the host position,
// the order Akkalat creates them in.
func (m Mesh) TileLocation(index int) [3]int {
	if index < 1 || index > m.NumTiles() {
		panic("waferscale: tile index out of range")
	}

	host := m.HostLocation()
	pos := index - 1
	if pos >= host[1]*m.Width+host[0] {
		pos++
	}

	return [3]int{pos % m.Width, pos / m.Width, 0}
}

// IOMMUPoolNodes sizes the IOMMU page-table pool in 4 KiB nodes so that it
// can map the memory of all the tiles densely. With 8-byte PTEs, the leaf
// tables take 8 bytes per page; one third more leaves room for the upper
// levels and some sparsity. The default 5x5 mesh of 4 GiB tiles with 4 KiB
// pages maps 96 GiB with 192 MiB of leaves and gets 65_536 nodes (256 MiB).
func IOMMUPoolNodes(m Mesh, tile Tile, log2PageSize uint64) uint64 {
	const pteBytes, nodeBytes = 8, 4096

	numPages := uint64(m.NumTiles()) * (tile.DRAMSize >> log2PageSize)
	leafBytes := numPages * pteBytes

	return (leafBytes*4/3 + nodeBytes - 1) / nodeBytes
}

// Tile microarchitectures.
const (
	TileR9Nano  = "r9nano"
	DefaultTile = TileR9Nano
)

// Tile holds the parameters of one GPU tile that the platform needs.
type Tile struct {
	NumCUPerShaderArray int    `json:"numCUPerShaderArray"`
	NumShaderArray      int    `json:"numShaderArray"`
	L2CacheSize         uint64 `json:"l2CacheSize"`
	DRAMSize            uint64 `json:"dramSize"`
}

// R9NanoTile is Akkalat's R9 Nano-based tile: 8 x 4 = 32 CUs and 4 GB.
var R9NanoTile = Tile{
	NumCUPerShaderArray: 4,
	NumShaderArray:      8,
	L2CacheSize:         4 * mem.MB,
	DRAMSize:            4 * mem.GB,
}

// ParseGPUType reports whether gpuType selects a wafer-scale GPU and, if so,
// the name of its tile microarchitecture.
func ParseGPUType(gpuType string) (tile string, ok bool) {
	if gpuType == GPUType {
		return DefaultTile, true
	}

	return strings.CutPrefix(gpuType, GPUType+"-")
}

// TileByName returns the parameters of the named tile microarchitecture.
func TileByName(name string) Tile {
	switch name {
	case TileR9Nano:
		return R9NanoTile
	default:
		panic(fmt.Sprintf("waferscale: unknown tile %q", name))
	}
}

// MakeR9NanoTileBuilder returns an R9 Nano builder configured as one tile.
func MakeR9NanoTileBuilder() r9nano.Builder {
	t := R9NanoTile

	return r9nano.MakeBuilder().
		WithNumCUPerShaderArray(t.NumCUPerShaderArray).
		WithNumShaderArray(t.NumShaderArray).
		WithL2CacheSize(t.L2CacheSize).
		WithDramSize(t.DRAMSize)
}
