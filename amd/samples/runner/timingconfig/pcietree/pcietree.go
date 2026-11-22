// Package pcietree assembles a PCIe tree whose devices can sit behind local
// switches. It is a thin layer over Akita's networkconnector, shaped like the
// local device groups proposed in sarchlab/akita#619.
package pcietree

import (
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Tree is a network of a root complex, switches, and devices at 1 GHz.
type Tree struct {
	connector *networkconnector.Connector
}

// New creates an empty tree with the given flit size in bytes.
func New(s modeling.Simulation, name string, flitSize int) *Tree {
	return &Tree{connector: networkconnector.MakeBuilder().
		WithSimulation(s).
		WithFreq(timing.GHz).
		WithFlitSize(flitSize).
		Build(name)}
}

// RootConfig configures the root complex.
type RootConfig struct {
	// Latency is the cycles that a flit pays when it enters the root complex.
	Latency int
}

// LocalGroupConfig configures a switch below another switch.
type LocalGroupConfig struct {
	Name string
	// Latency is the cycles that a flit pays when it enters this switch.
	Latency int
	// UplinkWidth is the flits per cycle of the link to the parent switch.
	UplinkWidth int
}

// Group is a switch of the tree that devices and other groups attach to.
type Group struct {
	tree    *Tree
	id      int
	latency int
}

// AddRootComplex adds the root of the tree.
func (t *Tree) AddRootComplex(config RootConfig) Group {
	id := t.connector.AddSwitch(networkconnector.SwitchConfig{Name: "RootComplex"})

	return Group{tree: t, id: id, latency: config.Latency}
}

// AddLocalGroup adds a switch with one uplink to parent. A flit pays the
// latency of the switch that it enters, in either direction.
func (t *Tree) AddLocalGroup(parent Group, config LocalGroupConfig) Group {
	id := t.connector.AddSwitch(networkconnector.SwitchConfig{Name: config.Name})
	t.connector.ConnectSwitches(id, parent.id, networkconnector.SwitchToSwitchLinkParameter{
		LeftEndParam:  switchEnd(config.UplinkWidth, config.Latency),
		RightEndParam: switchEnd(config.UplinkWidth, parent.latency),
	})

	return Group{tree: t, id: id, latency: config.Latency}
}

// ConnectDevice attaches ports to the group through one endpoint with a link
// of the given width. Ports on an endpoint can block each other's delivery, so
// ports with a request/response dependency need separate endpoints.
func (g Group) ConnectDevice(name string, width int, ports []messaging.Port) {
	g.tree.connector.ConnectDevice(g.id, ports, networkconnector.DeviceConfig{
		Name:           name,
		DeviceEndParam: networkconnector.LinkEndDeviceParameter{InputWidth: width, OutputWidth: width},
		SwitchEndParam: switchEnd(width, g.latency),
	})
}

// ConnectPeers adds a one-flit-wide non-tree link between two groups, such as
// Infinity Fabric. A flit pays latency when it crosses the link, in place of
// the latency of the group that it enters.
func ConnectPeers(a, b Group, latency int) {
	a.tree.connector.ConnectSwitches(a.id, b.id, networkconnector.SwitchToSwitchLinkParameter{
		LeftEndParam:  switchEnd(1, latency),
		RightEndParam: switchEnd(1, latency),
	})
}

// HostPorts collects the ports of the host until the tree is connected.
type HostPorts struct {
	memory []messaging.Port
	others []messaging.Port
}

// BindMemory adds a host-memory port, which gets a link as wide as the fabric.
func (h *HostPorts) BindMemory(port messaging.Port) { h.memory = append(h.memory, port) }

// Bind adds any other host port, which gets a one-flit-wide link.
func (h *HostPorts) Bind(port messaging.Port) { h.others = append(h.others, port) }

// Connect adds the zero-latency Host Fabric below root and gives every host
// port its own endpoint on it. Host-memory latency includes the fabric.
func (h *HostPorts) Connect(tree *Tree, root Group, width int) {
	fabric := tree.AddLocalGroup(root, LocalGroupConfig{Name: "HostFabric", UplinkWidth: width})
	for _, port := range h.memory {
		fabric.ConnectDevice(endpointName(port), width, []messaging.Port{port})
	}
	for _, port := range h.others {
		fabric.ConnectDevice(endpointName(port), 1, []messaging.Port{port})
	}
}

func endpointName(port messaging.Port) string {
	return "HostFabric." + port.Name() + ".EndPoint"
}

func switchEnd(width, latency int) networkconnector.LinkEndSwitchParameter {
	return networkconnector.LinkEndSwitchParameter{InputWidth: width, OutputWidth: width, Latency: latency}
}
