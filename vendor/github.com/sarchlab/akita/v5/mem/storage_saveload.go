package mem

import (
	"encoding/binary"
	"io"
)

// Save writes the storage contents to w in a binary format.
//
// Format (all values little-endian):
//
//	capacity  uint64
//	unitSize  uint64
//	count     uint64   (number of allocated storage units)
//	For each allocated unit:
//	  baseAddr  uint64
//	  data      [unitSize]byte
func (s *Storage) Save(w io.Writer) error {
	s.Lock()
	defer s.Unlock()

	if err := binary.Write(w, binary.LittleEndian, s.Capacity); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, s.unitSize); err != nil {
		return err
	}

	count := uint64(len(s.data))
	if err := binary.Write(w, binary.LittleEndian, count); err != nil {
		return err
	}

	for baseAddr, unit := range s.data {
		if err := binary.Write(w, binary.LittleEndian, baseAddr); err != nil {
			return err
		}

		if _, err := w.Write(unit.data); err != nil {
			return err
		}
	}

	return nil
}

// Load reads storage contents from r, replacing the current contents.
//
// The binary format is described in [Storage.Save].
func (s *Storage) Load(r io.Reader) error {
	s.Lock()
	defer s.Unlock()

	var capacity, unitSize, count uint64

	if err := binary.Read(r, binary.LittleEndian, &capacity); err != nil {
		return err
	}

	if err := binary.Read(r, binary.LittleEndian, &unitSize); err != nil {
		return err
	}

	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}

	s.Capacity = capacity
	s.unitSize = unitSize
	s.data = make(map[uint64]*storageUnit, count)

	for i := uint64(0); i < count; i++ {
		var baseAddr uint64
		if err := binary.Read(r, binary.LittleEndian, &baseAddr); err != nil {
			return err
		}

		unit := &storageUnit{
			data: make([]byte, unitSize),
		}

		if _, err := io.ReadFull(r, unit.data); err != nil {
			return err
		}

		s.data[baseAddr] = unit
	}

	return nil
}
