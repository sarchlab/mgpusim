// Package domain provides a simple Domain type that replaces sim.Domain
// which was removed in V5.
package domain

import "github.com/sarchlab/akita/v5/sim"

// Domain groups named ports for hierarchical GPU component composition.
// TODO: V5 migration - sim.Domain was removed. This is a local replacement.
type Domain struct {
	name  string
	ports map[string]sim.Port
}

// NewDomain creates a new Domain with the given name.
func NewDomain(name string) *Domain {
	return &Domain{
		name:  name,
		ports: make(map[string]sim.Port),
	}
}

// Name returns the domain name.
func (d *Domain) Name() string {
	return d.name
}

// AddPort registers a named port.
func (d *Domain) AddPort(name string, port sim.Port) {
	d.ports[name] = port
}

// GetPortByName returns the port with the given name, or nil.
func (d *Domain) GetPortByName(name string) sim.Port {
	return d.ports[name]
}

// Ports returns all registered ports.
func (d *Domain) Ports() map[string]sim.Port {
	return d.ports
}
