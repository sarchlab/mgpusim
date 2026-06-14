package modeling

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
)

// Domain is a named bundle of components. It exposes a subset of its internal
// components' ports at the domain boundary, so the outside world communicates
// with the domain without knowing its internal structure. Domains nest:
// components form a domain (e.g., a shader array), and domains compose into
// larger domains (e.g., a GPU), with hierarchical names following the
// "Domain.Domain.Component" convention.
//
// A domain declares its boundary ports with DeclarePort and exposes an
// internal component's port with AssignPort, following the same declare/assign
// idiom as components.
type Domain struct {
	*messaging.PortOwnerBase

	name string
}

// NewDomain creates a new Domain with the given hierarchical name.
func NewDomain(name string) *Domain {
	naming.MustBeValid(name)

	return &Domain{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}
}

// Name returns the name of the domain.
func (d *Domain) Name() string {
	return d.name
}
