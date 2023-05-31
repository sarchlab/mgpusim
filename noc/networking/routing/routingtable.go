package routing

import "github.com/sarchlab/akita/v3/sim"

// Table is a routing table that can find the next-hop port according to the
// final destination.
type Table interface {
	FindPort(dst sim.Port) sim.Port
	DefineRoute(finalDst, outputPort sim.Port)
	DefineDefaultRoute(outputPort sim.Port)
}

// NewTable creates a new Table.
func NewTable() Table {
	t := &table{}
	t.t = make(map[sim.Port]sim.Port)
	return t
}

type table struct {
	t           map[sim.Port]sim.Port
	defaultPort sim.Port
}

func (t table) FindPort(dst sim.Port) sim.Port {
	out, found := t.t[dst]
	if found {
		return out
	}
	return t.defaultPort
}

func (t *table) DefineRoute(finalDst, outputPort sim.Port) {
	t.t[finalDst] = outputPort
}

func (t *table) DefineDefaultRoute(outputPort sim.Port) {
	t.defaultPort = outputPort
}
