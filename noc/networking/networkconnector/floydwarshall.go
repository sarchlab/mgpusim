package networkconnector

import (
	"fmt"
)

// FloydWarshallRouter is a simple router that always establish route that
// involves the least number of hops.
type FloydWarshallRouter struct{}

type routeInfo struct {
	src      Node
	dst      Node
	distance uint32
	nextHop  *Remote
}

// EstablishRoute creates routes for the tables.
func (r FloydWarshallRouter) EstablishRoute(nodes []Node) {
	table := make([][]routeInfo, len(nodes))
	r.floydWarshallInit(table, nodes)
	// r.dumpTable(table)

	r.floydWarshall(table)
	// r.dumpTable(table)

	r.tableToRoute(table, nodes)
}

func (r FloydWarshallRouter) floydWarshallInit(table [][]routeInfo, nodes []Node) {
	for i := range table {
		table[i] = make([]routeInfo, len(nodes))
		for j := range table[i] {
			table[i][j].src = nodes[i]
			table[i][j].dst = nodes[j]
			table[i][j].distance = uint32(2 * len(nodes))

			remotes := nodes[i].ListRemotes()

			if i == j {
				table[i][j].distance = 0
				table[i][j].nextHop = &remotes[0]
				continue
			}

			r := findRemote(remotes, table[i][j].dst)
			if r != nil {
				table[i][j].distance = 1
				table[i][j].nextHop = r
			}

			// fmt.Printf("FW Init Internal %d/%d\n", j, len(table))
		}

		// fmt.Printf("FW Init %d/%d\n", i, len(table))
	}
}

func (r FloydWarshallRouter) floydWarshall(table [][]routeInfo) {
	for k := range table {
		for i := range table {
			for j := range table {
				originalDist := table[i][j].distance
				newDist := table[i][k].distance + table[k][j].distance

				if newDist < originalDist {
					table[i][j].distance = newDist
					table[i][j].nextHop = table[i][k].nextHop
				}
			}
		}

		// fmt.Printf("FW %d/%d\n", k, len(table))
	}
}

func (r FloydWarshallRouter) tableToRoute(table [][]routeInfo, nodes []Node) {
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

func (r FloydWarshallRouter) dumpTable(table [][]routeInfo) {
	fmt.Println("")
	for i := range table {
		for j := range table[i] {
			cell := ""
			cell += fmt.Sprintf("%d ", table[i][j].distance)
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

func findRemote(l []Remote, t Node) (r *Remote) {
	for _, r := range l {
		if t == r.RemoteNode {
			return &r
		}
	}

	return nil
}
