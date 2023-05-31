package mesh

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/networking/networkconnector"
)

// A meshRouter is a routing table that particularly works with mesh networks.
type meshRouter struct {
	x, y, z int
}

// EstablishRoute creates routes for the tables.
func (r meshRouter) EstablishRoute(nodes []networkconnector.Node) {
	eps := make([]networkconnector.Node, 0, len(nodes)/2)
	sws := make([]networkconnector.Node, 0, len(nodes)/2)

	for _, n := range nodes {
		tokens := strings.Split(n.Name(), ".")

		switch {
		case strings.Contains(tokens[1], "EP"):
			eps = append(eps, n)
		case strings.Contains(tokens[1], "SW"):
			sws = append(sws, n)
		}
	}

	wg := &sync.WaitGroup{}
	for i, sw := range sws {
		wg.Add(1)
		go func(sw networkconnector.Node, i int) {
			r.RouteSwitch(sw, eps)
			fmt.Printf("Routing table created %d/%d\n", i+1, len(sws))
			wg.Done()
		}(sw, i)
	}
	wg.Wait()
}

// RouteSwitch create a routing table for a switch.
func (r meshRouter) RouteSwitch(
	n networkconnector.Node,
	eps []networkconnector.Node,
) {
	top, bottom, left, right, front, back := r.getPortOnEachDirection(n)
	r.createRouteForEndPoints(eps, top, bottom, left, right, front, back, n)
}

func (r meshRouter) getPortOnEachDirection(n networkconnector.Node) (
	top, bottom, left, right, front, back sim.Port,
) {
	swX, swY, swZ := r.epNameToCoordinate(n.Name())

	remotes := n.ListRemotes()
	for _, remote := range remotes {
		remoteName := remote.RemoteNode.Name()
		if strings.Contains(remoteName, "EP") {
			n.Table().DefineDefaultRoute(remote.LocalPort)
			continue
		}

		remoteX, remoteY, remoteZ := r.epNameToCoordinate(remoteName)

		switch {
		case remoteZ == swZ+1:
			front = remote.LocalPort
		case remoteZ == swZ-1:
			back = remote.LocalPort
		case remoteY == swY+1:
			top = remote.LocalPort
		case remoteY == swY-1:
			bottom = remote.LocalPort
		case remoteX == swX+1:
			right = remote.LocalPort
		case remoteX == swX-1:
			left = remote.LocalPort
		default:
			msg := fmt.Sprintf("unexpected remote: %s, current node %s",
				remoteName, n.Name())
			panic(msg)
		}
	}
	return top, bottom, left, right, front, back
}

func (r meshRouter) createRouteForEndPoints(
	eps []networkconnector.Node,
	top, bottom, left, right, front, back sim.Port,
	n networkconnector.Node,
) {
	swX, swY, swZ := r.epNameToCoordinate(n.Name())
	for _, ep := range eps {
		epPort := ep.ListRemotes()[0].LocalPort
		epX, epY, epZ := r.epNameToCoordinate(ep.Name())

		nextHopPort := r.epCoordToForwardPort(
			epX, epY, epZ,
			swX, swY, swZ,
			top, bottom, left, right, front, back, epPort,
		)

		// fmt.Printf("%s -> %s -> ... -> %s\n",
		// 	n.Name(), nextHopPort.Name(), epPort.Name())

		n.Table().DefineRoute(epPort, nextHopPort)
	}
}

func (meshRouter) epCoordToForwardPort(
	epX, epY, epZ int,
	swX, swY, swZ int,
	top, bottom, left, right, front, back, localEPPort sim.Port,
) sim.Port {
	var nextHopPort sim.Port
	switch {
	case epZ < swZ:
		nextHopPort = back
	case epZ > swZ:
		nextHopPort = front
	case epY < swY:
		nextHopPort = bottom
	case epY > swY:
		nextHopPort = top
	case epX < swX:
		nextHopPort = left
	case epX > swX:
		nextHopPort = right
	case epX == swX && epY == swY && epZ == swZ:
		nextHopPort = localEPPort
	default:
		panic("unknown endpoint")
	}
	return nextHopPort
}

func (r meshRouter) epNameToCoordinate(
	epName string,
) (int, int, int) {
	tokens := strings.Split(epName, "_")
	x, _ := strconv.Atoi(tokens[1])
	y, _ := strconv.Atoi(tokens[2])
	z, _ := strconv.Atoi(tokens[3])

	return x, y, z
}
