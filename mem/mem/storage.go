package mem

import (
	"errors"
	"sync"
)

// For capacity
const (
	_         = iota
	KB uint64 = 1 << (10 * iota)
	MB
	GB
	TB
)

// A Storage keeps the data of the guest system.
//
// A storage is an abstraction of all different type of storage including
// registers, main memory, and hard drives.
//
// The storage implementation manages the storage in units. The unit can is
// similar to the concept of page in memory management. For the units that
// it not touched by Read and Write function, no memory will be allocated.
//
type Storage struct {
	sync.Mutex
	Capacity uint64
	unitSize uint64
	data     map[uint64]*storageUnit
}

type storageUnit struct {
	sync.RWMutex
	data []byte
}

func newStorageUnit(uintSize uint64) *storageUnit {
	u := new(storageUnit)
	u.data = make([]byte, uintSize)
	return u
}

// NewStorage creates a storage object with the specified capacity
func NewStorage(capacity uint64) *Storage {
	storage := new(Storage)

	storage.Capacity = capacity
	storage.unitSize = 4 * KB
	storage.data = make(map[uint64]*storageUnit)

	return storage
}

// NewStorageWithUnitSize creates a storage object with the specified capacity.
// The unit size is specified in bytes. Using unit size can reduces the memory
// consumption of storage.
func NewStorageWithUnitSize(capacity uint64, unitSize uint64) *Storage {
	storage := new(Storage)

	storage.Capacity = capacity
	storage.unitSize = unitSize
	storage.data = make(map[uint64]*storageUnit)

	return storage
}

// createOrGetStorageUnit retrieves a storage unit if the unit has been created
// before. Otherwise it initializes a storage unit in the storage object
func (s *Storage) createOrGetStorageUnit(address uint64) (*storageUnit, error) {
	if address > s.Capacity {
		return nil, errors.New("accessing physical address beyond the storage capacity")
	}

	baseAddr, _ := s.parseAddress(address)
	s.Lock()
	unit, ok := s.data[baseAddr]
	if !ok {
		unit = newStorageUnit(s.unitSize)
		s.data[baseAddr] = unit
	}
	s.Unlock()
	return unit, nil
}

func (s *Storage) parseAddress(addr uint64) (baseAddr, inUnitAddr uint64) {
	inUnitAddr = addr % s.unitSize
	baseAddr = addr - inUnitAddr
	return
}

func (s *Storage) Read(address uint64, len uint64) ([]byte, error) {
	currAddr := address
	lenLeft := len
	dataOffset := uint64(0)
	res := make([]byte, len)

	for currAddr < address+len {
		unit, err := s.createOrGetStorageUnit(currAddr)
		if err != nil {
			return nil, err
		}

		baseAddr, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInUnit := baseAddr + s.unitSize - currAddr
		var lenToRead uint64
		if lenLeft < lenLeftInUnit {
			lenToRead = lenLeft
		} else {
			lenToRead = lenLeftInUnit
		}

		copy(res[dataOffset:dataOffset+lenToRead],
			unit.data[inUnitAddr:inUnitAddr+lenToRead])

		lenLeft -= lenToRead
		dataOffset += lenToRead
		currAddr += lenToRead
	}

	return res, nil
}

func (s *Storage) Write(address uint64, data []byte) error {
	currAddr := address
	dataOffset := uint64(0)

	for dataOffset < uint64(len(data)) {
		unit, err := s.createOrGetStorageUnit(currAddr)
		if err != nil {
			return err
		}

		_, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInData := uint64(len(data)) - dataOffset
		lenLeftInUnit := currAddr/s.unitSize*s.unitSize + s.unitSize - currAddr

		var lenToWrite uint64
		if lenLeftInData < lenLeftInUnit {
			lenToWrite = lenLeftInData
		} else {
			lenToWrite = lenLeftInUnit
		}

		copy(unit.data[inUnitAddr:inUnitAddr+lenToWrite],
			data[dataOffset:dataOffset+lenToWrite])

		dataOffset += lenToWrite
		currAddr += lenToWrite
	}

	return nil
}
