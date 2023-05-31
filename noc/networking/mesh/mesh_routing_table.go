package mesh

import (
	"github.com/sarchlab/akita/v3/sim"
)

// meshRoutingTable is a routing table that can find the next-hop port according
// to the coordinate of the final destination.
type meshRoutingTable struct {
	x, y, z                               int
	top, left, bottom, right, front, back sim.Port
	local                                 sim.Port
	dstTable                              map[string]*tile
}

// FindPort finds the next-hop port according to the coordinate of the final
// destination.
func (t *meshRoutingTable) FindPort(dst sim.Port) sim.Port {
	// fmt.Printf("dst name: %s\n", dst.Name())

	dstTile := t.dstTable[dst.Name()]
	dstX, dstY, dstZ := dstTile.rt.x, dstTile.rt.y, dstTile.rt.z

	// fmt.Printf("dst coord is: %d, %d, %d\n", dstX, dstY, dstZ)

	switch {
	case dstZ < t.z:
		return t.front
	case dstZ > t.z:
		return t.back
	case dstY < t.y:
		return t.top
	case dstY > t.y:
		return t.bottom
	case dstX < t.x:
		return t.left
	case dstX > t.x:
		return t.right
	case dstX == t.x && dstY == t.y && dstZ == t.z:
		return t.local
	default:
		panic("unreachable")
	}
}

// DefineRoute does noting
func (t *meshRoutingTable) DefineRoute(finalDst, outputPort sim.Port) {
	// Do nothing.
}

// DefineDefaultRoute sets the local port.
func (t *meshRoutingTable) DefineDefaultRoute(outputPort sim.Port) {
	t.local = outputPort
}
