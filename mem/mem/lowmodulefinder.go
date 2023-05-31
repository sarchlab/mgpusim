package mem

import "github.com/sarchlab/akita/v3/sim"

// LowModuleFinder helps a cache unit or a akita to find the low module that
// should hold the data at a certain address
type LowModuleFinder interface {
	Find(address uint64) sim.Port
}

// SingleLowModuleFinder is used when a unit is connected with only one
// low module
type SingleLowModuleFinder struct {
	LowModule sim.Port
}

// Find simply returns the solo unit that it connects to
func (f *SingleLowModuleFinder) Find(address uint64) sim.Port {
	return f.LowModule
}

// InterleavedLowModuleFinder helps find the low module when the low modules
// maintains interleaved address space
type InterleavedLowModuleFinder struct {
	UseAddressSpaceLimitation bool
	LowAddress                uint64
	HighAddress               uint64
	InterleavingSize          uint64
	LowModules                []sim.Port
	ModuleForOtherAddresses   sim.Port
}

// Find returns the low module that has the data at provided address
func (f *InterleavedLowModuleFinder) Find(address uint64) sim.Port {
	if f.UseAddressSpaceLimitation &&
		(address >= f.HighAddress || address < f.LowAddress) {
		return f.ModuleForOtherAddresses
	}
	number := address / f.InterleavingSize % uint64(len(f.LowModules))
	return f.LowModules[number]
}

// NewInterleavedLowModuleFinder creates a new finder for interleaved lower
// modules
func NewInterleavedLowModuleFinder(interleavingSize uint64) *InterleavedLowModuleFinder {
	finder := new(InterleavedLowModuleFinder)
	finder.LowModules = make([]sim.Port, 0)
	finder.InterleavingSize = interleavingSize
	return finder
}

// BankedLowModuleFinder defines the lower level modules by address banks
type BankedLowModuleFinder struct {
	BankSize   uint64
	LowModules []sim.Port
}

// Find returns the port that can provide the data.
func (f *BankedLowModuleFinder) Find(address uint64) sim.Port {
	i := address / f.BankSize
	return f.LowModules[i]
}

// NewBankedLowModuleFinder returns a new BankedLowModuleFinder.
func NewBankedLowModuleFinder(bankSize uint64) *BankedLowModuleFinder {
	f := new(BankedLowModuleFinder)
	f.BankSize = bankSize
	f.LowModules = make([]sim.Port, 0)
	return f
}
