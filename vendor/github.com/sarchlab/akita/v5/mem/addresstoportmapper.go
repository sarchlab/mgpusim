package mem

import "github.com/sarchlab/akita/v5/sim"

// AddressToPortMapper helps a cache unit or a akita to find the low module that
// should hold the data at a certain address
type AddressToPortMapper interface {
	Find(address uint64) sim.RemotePort
}

// SinglePortMapper is used when a unit is connected with only one
// low module
type SinglePortMapper struct {
	Port sim.RemotePort
}

// Find simply returns the solo unit that it connects to
func (f *SinglePortMapper) Find(address uint64) sim.RemotePort {
	return f.Port
}

// InterleavedAddressPortMapper helps find the low module when the low modules
// maintains interleaved address space
type InterleavedAddressPortMapper struct {
	UseAddressSpaceLimitation bool
	LowAddress                uint64
	HighAddress               uint64
	InterleavingSize          uint64
	LowModules                []sim.RemotePort
	ModuleForOtherAddresses   sim.RemotePort
}

// Find returns the low module that has the data at provided address
func (f *InterleavedAddressPortMapper) Find(address uint64) sim.RemotePort {
	if f.UseAddressSpaceLimitation &&
		(address >= f.HighAddress || address < f.LowAddress) {
		return f.ModuleForOtherAddresses
	}

	number := address / f.InterleavingSize % uint64(len(f.LowModules))

	return f.LowModules[number]
}

// NewInterleavedAddressPortMapper creates a new finder for interleaved lower
// modules
func NewInterleavedAddressPortMapper(
	interleavingSize uint64,
) *InterleavedAddressPortMapper {
	finder := new(InterleavedAddressPortMapper)

	finder.LowModules = make([]sim.RemotePort, 0)
	finder.InterleavingSize = interleavingSize

	return finder
}

// BankedAddressPortMapper defines the lower level modules by address banks
type BankedAddressPortMapper struct {
	BankSize   uint64
	LowModules []sim.RemotePort
}

// Find returns the port that can provide the data.
func (f *BankedAddressPortMapper) Find(address uint64) sim.RemotePort {
	i := address / f.BankSize
	return f.LowModules[i]
}

// NewBankedAddressPortMapper returns a new BankedAddressToPortMapper.
func NewBankedAddressPortMapper(bankSize uint64) *BankedAddressPortMapper {
	f := new(BankedAddressPortMapper)
	f.BankSize = bankSize
	f.LowModules = make([]sim.RemotePort, 0)

	return f
}
