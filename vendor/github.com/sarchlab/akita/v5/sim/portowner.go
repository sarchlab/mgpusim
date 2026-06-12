package sim

import (
	"fmt"
	"os"
	"sort"
)

// A PortOwner is an element that can communicate with others through ports.
type PortOwner interface {
	AddPort(name string, port Port)
	GetPortByName(name string) Port
	Ports() []Port
}

// PortOwnerBase provides an implementation of the PortOwner interface.
type PortOwnerBase struct {
	ports map[string]Port
}

// NewPortOwnerBase creates a new PortOwnerBase
func NewPortOwnerBase() *PortOwnerBase {
	return &PortOwnerBase{
		ports: make(map[string]Port),
	}
}

// AddPort adds a new port with a given name.
func (po *PortOwnerBase) AddPort(name string, port Port) {
	if _, found := po.ports[name]; found {
		panic("port already exist")
	}

	po.ports[name] = port
}

// GetPortByName returns the port according to the name of the port. This
// function panics when the given name is not found.
func (po PortOwnerBase) GetPortByName(name string) Port {
	port, found := po.ports[name]
	if !found {
		errMsg := fmt.Sprintf(
			"Port %s is not available.\n", name)
		errMsg += "Available ports include:\n"

		for n := range po.ports {
			errMsg += fmt.Sprintf("\t%s\n", n)
		}

		fmt.Fprint(os.Stderr, errMsg)

		panic("port not found")
	}

	return port
}

// Ports returns a slices of all the ports owned by the PortOwner.
func (po PortOwnerBase) Ports() []Port {
	portList := make([]string, 0, len(po.ports))

	for k := range po.ports {
		portList = append(portList, k)
	}

	sort.Strings(portList)

	list := make([]Port, 0, len(po.ports))

	for _, port := range portList {
		list = append(list, po.ports[port])
	}

	return list
}
