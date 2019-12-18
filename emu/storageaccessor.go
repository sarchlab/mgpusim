package emu

import (
	"log"

	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/ca"
)

type storageAccessor struct {
	storage       *mem.Storage
	addrConverter idealmemcontroller.AddressConverter
	pageTable     vm.PageTable
	log2PageSize  uint64
}

func (a *storageAccessor) Read(pid ca.PID, vAddr, byteSize uint64) []byte {
	data := make([]byte, byteSize)
	sizeLeft := byteSize
	offset := uint64(0)

	for sizeLeft > 0 {
		currVAddr := vAddr + offset
		nextPageStart := ((currVAddr >> a.log2PageSize) + 1) << a.log2PageSize
		sizeInPageLeft := nextPageStart - currVAddr
		sizeToRead := sizeInPageLeft
		if sizeToRead > sizeLeft {
			sizeToRead = sizeLeft
		}

		page, found := a.pageTable.Find(pid, currVAddr)
		if !found {
			panic("page not found in page table")
		}
		pAddr := page.PAddr + (currVAddr - page.VAddr)

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
	sizeLeft := uint64(len(data))
	offset := uint64(0)

	for sizeLeft > 0 {
		currVAddr := vAddr + offset
		nextPageStart := ((currVAddr >> a.log2PageSize) + 1) << a.log2PageSize
		sizeInPageLeft := nextPageStart - currVAddr
		sizeToWrite := sizeInPageLeft
		if sizeToWrite > sizeLeft {
			sizeToWrite = sizeLeft
		}

		page, found := a.pageTable.Find(pid, vAddr)
		if !found {
			panic("page not found in page table")
		}
		pAddr := page.PAddr + (currVAddr - page.VAddr)

		storageAddr := pAddr
		if a.addrConverter != nil {
			storageAddr = a.addrConverter.ConvertExternalToInternal(pAddr)
		}

		err := a.storage.Write(storageAddr, data[offset:offset+sizeToWrite])
		if err != nil {
			log.Panic(err)
		}

		offset += sizeToWrite
		sizeLeft -= sizeToWrite
	}
}

// NewStorageAccessor creates a storageAccessor, injecting dependencies
// of the storage and mmu.
func newStorageAccessor(
	storage *mem.Storage,
	pageTable vm.PageTable,
	log2PageSize uint64,
	addrConverter idealmemcontroller.AddressConverter,
) *storageAccessor {
	a := new(storageAccessor)
	a.storage = storage
	a.addrConverter = addrConverter
	a.pageTable = pageTable
	a.log2PageSize = log2PageSize
	return a
}
