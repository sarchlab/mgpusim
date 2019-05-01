package emu

import (
	"log"

	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

type storageAccessor struct {
	storage       *mem.Storage
	addrConverter mem.AddressConverter
	mmu           vm.MMU
}

func (a *storageAccessor) Read(pid ca.PID, vAddr, byteSize uint64) []byte {
	phyAddr, page := a.mmu.Translate(pid, vAddr)
	if page == nil {
		log.Panic("page not found in page table")
	}

	//fmt.Printf("pid: %d, va: 0x%x, pa: 0x%x\n", pid, vAddr, phyAddr)

	storageAddr := a.addrConverter.ConvertExternalToInternal(phyAddr)
	data, err := a.storage.Read(storageAddr, byteSize)
	if err != nil {
		log.Panic(err)
	}

	return data
}

func (a *storageAccessor) Write(pid ca.PID, vAddr uint64, data []byte) {
	phyAddr, page := a.mmu.Translate(pid, vAddr)
	if page == nil {
		log.Panic("page not found in page table")
	}

	storageAddr := a.addrConverter.ConvertExternalToInternal(phyAddr)
	err := a.storage.Write(storageAddr, data)
	if err != nil {
		log.Panic(err)
	}
}

// NewStorageAccessor creates a storageAccessor, injecting dependencies
// of the storage and mmu.
func newStorageAccessor(
	storage *mem.Storage,
	mmu vm.MMU,
	addrConverter mem.AddressConverter,
) *storageAccessor {
	a := new(storageAccessor)
	a.storage = storage
	a.addrConverter = addrConverter
	a.mmu = mmu
	return a
}
