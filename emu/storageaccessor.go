package emu

import (
	"log"

	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/util/ca"
)

type storageAccessor struct {
	storage       *mem.Storage
	addrConverter idealmemcontroller.AddressConverter
	mmu           mmu.MMU
}

func (a *storageAccessor) Read(pid ca.PID, vAddr, byteSize uint64) []byte {
	data := make([]byte, byteSize)
	sizeLeft := byteSize
	offset := uint64(0)
	log2PageSize := uint64(12)
	// pageSize := uint64(1 << log2PageSize)

	for sizeLeft > 0 {
		currVAddr := vAddr + offset
		nextPageStart := ((currVAddr >> log2PageSize) + 1) << log2PageSize
		sizeInPageLeft := nextPageStart - currVAddr
		sizeToRead := sizeInPageLeft
		if sizeToRead > sizeLeft {
			sizeToRead = sizeLeft
		}

		pAddr, page := a.mmu.Translate(pid, currVAddr)
		if page == nil {
			log.Panic("page not found in page table")
		}

		storageAddr := pAddr
		if a.addrConverter != nil {
			storageAddr = a.addrConverter.ConvertExternalToInternal(pAddr)
		}

		d, err := a.storage.Read(storageAddr, sizeToRead)
		if err != nil {
			log.Panic(err)
		}

		copy(data[offset:], d)

		offset += sizeToRead
		sizeLeft -= sizeToRead
	}

	return data
}

func (a *storageAccessor) Write(pid ca.PID, vAddr uint64, data []byte) {
	phyAddr, page := a.mmu.Translate(pid, vAddr)
	if page == nil {
		log.Panic("page not found in page table")
	}

	storageAddr := phyAddr
	if a.addrConverter != nil {
		storageAddr = a.addrConverter.ConvertExternalToInternal(phyAddr)
	}
	err := a.storage.Write(storageAddr, data)
	if err != nil {
		log.Panic(err)
	}

	// log.Printf("write, %d, %d, %d, %v", pid, vAddr, phyAddr, data)
}

// NewStorageAccessor creates a storageAccessor, injecting dependencies
// of the storage and mmu.
func newStorageAccessor(
	storage *mem.Storage,
	mmu mmu.MMU,
	addrConverter idealmemcontroller.AddressConverter,
) *storageAccessor {
	a := new(storageAccessor)
	a.storage = storage
	a.addrConverter = addrConverter
	a.mmu = mmu
	return a
}
