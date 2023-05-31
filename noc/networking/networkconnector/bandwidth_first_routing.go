package networkconnector

import (
	"fmt"
	"math"
)

// BandwidthFirstRouter is a simple router that always establish route that
// involves the least number of hops.
type BandwidthFirstRouter struct {
	FlitSize int
}

type bfRouteInfo struct {
	src       Node
	dst       Node
	bandwidth float64
	nextHop   *Remote
}

// EstablishRoute creates routes for the tables.
func (r BandwidthFirstRouter) EstablishRoute(nodes []Node) {
	table := make([][]bfRouteInfo, len(nodes))
	r.floydWarshallInit(table, nodes)
	// r.dumpTable(table)

	r.floydWarshall(table)
	// r.dumpTable(table)

	r.tableToRoute(table, nodes)
}

func (r BandwidthFirstRouter) floydWarshallInit(
	table [][]bfRouteInfo,
	nodes []Node,
) {
	for i := range table {
		table[i] = make([]bfRouteInfo, len(nodes))
		for j := range table[i] {
			table[i][j].src = nodes[i]
			table[i][j].dst = nodes[j]
			table[i][j].bandwidth = -1

			remotes := nodes[i].ListRemotes()

			if i == j {
				table[i][j].bandwidth = math.Inf(1)
				table[i][j].nextHop = &remotes[0]
				continue
			}

			remote := findRemote(remotes, table[i][j].dst)
			if remote != nil {
				table[i][j].bandwidth = remote.Bandwidth(r.FlitSize)
				table[i][j].nextHop = remote
			}
		}
	}
}

func (r BandwidthFirstRouter) floydWarshall(table [][]bfRouteInfo) {
	for k := range table {
		for i := range table {
			for j := range table {
				originalBW := table[i][j].bandwidth
				newBW := r.min(table[i][k].bandwidth, table[k][j].bandwidth)

				if newBW > originalBW {
					table[i][j].bandwidth = newBW
					table[i][j].nextHop = table[i][k].nextHop
				}
			}
		}
	}
}

func (r BandwidthFirstRouter) tableToRoute(table [][]bfRouteInfo, nodes []Node) {
	for i, n1 := range nodes {
		swNode, isSwitch := n1.(*switchNode)
		if !isSwitch {
			continue
		}

		for j, n2 := range nodes {
			epNode, isEp := n2.(*deviceNode)
			if !isEp {
				continue
			}

			remote := table[i][j].nextHop
			for _, p := range epNode.ports {
				swNode.Table().DefineRoute(p, remote.LocalPort)
			}
		}
	}
}

func (r BandwidthFirstRouter) dumpTable(table [][]bfRouteInfo) {
	fmt.Println("")
	for i := range table {
		for j := range table[i] {
			cell := ""
			cell += fmt.Sprintf("%f ", table[i][j].bandwidth)

			if table[i][j].nextHop != nil {
				cell += fmt.Sprintf("%s->%s->%s\t",
					table[i][j].src.Name(),
					table[i][j].nextHop.RemoteNode.Name(),
					table[i][j].dst.Name(),
				)
			} else {
				cell += fmt.Sprintf("%s->XXX->%s\t",
					table[i][j].src.Name(),
					table[i][j].dst.Name(),
				)
			}

			for i := len(cell); i < 100; i++ {
				cell += " "
			}

			fmt.Print(cell)
		}
		fmt.Printf("\n")
	}
}

func (r BandwidthFirstRouter) min(a, b float64) float64 {
	if a < b {
		return a
	}

	return b
}
