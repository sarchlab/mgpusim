package emu

import (
	"log"

	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
)

type storageAccessor struct {
	storage *mem.Storage
	mmu     vm.MMU
}

func (a *storageAccessor) Read(pid vm.PID, vAddr, byteSize uint64) []byte {
	phyAddr, found := a.mmu.Translate(pid, vAddr)
	if !found {
		log.Panic("page not found in page table")
	}

	//fmt.Printf("pid: %d, va: 0x%x, pa: 0x%x\n", pid, vAddr, phyAddr)

	data, err := a.storage.Read(phyAddr, byteSize)
	if err != nil {
		log.Panic(err)
	}

	return data
}

func (a *storageAccessor) Write(pid vm.PID, vAddr uint64, data []byte) {
	phyAddr, found := a.mmu.Translate(pid, vAddr)
	if !found {
		log.Panic("page not found in page table")
	}

	err := a.storage.Write(phyAddr, data)
	if err != nil {
		log.Panic(err)
	}
}

// NewStorageAccessor creates a storageAccessor, injecting dependencies
// of the storage and mmu.
func newStorageAccessor(storage *mem.Storage, mmu vm.MMU) *storageAccessor {
	a := new(storageAccessor)
	a.storage = storage
	a.mmu = mmu
	return a
}
