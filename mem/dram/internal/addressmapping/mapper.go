// Package addressmapping defines how to maps an address to a localtion.
package addressmapping

// Mapper can map from an address to a location.
type Mapper interface {
	Map(addr uint64) Location
}
