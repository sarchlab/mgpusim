package mem

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

// SaveCheckpoint writes the storage shape (capacity, unit size) and every
// allocated unit, sorted by address, as a compact binary stream. Untouched units
// are never allocated and therefore not written, so a sparse storage stays small.
func (s *Storage) SaveCheckpoint(w io.Writer) error {
	s.Lock()
	defer s.Unlock()

	addrs := make([]uint64, 0, len(s.data))
	for addr := range s.data {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

	if err := writeUint64(w, s.capacity); err != nil {
		return err
	}
	if err := writeUint64(w, s.unitSize); err != nil {
		return err
	}
	if err := writeUint64(w, uint64(len(addrs))); err != nil {
		return err
	}

	for _, addr := range addrs {
		if err := writeUint64(w, addr); err != nil {
			return err
		}
		if _, err := w.Write(s.data[addr].data); err != nil {
			return err
		}
	}

	return nil
}

// LoadCheckpoint restores the allocated units after checking that the saved shape
// matches the rebuilt storage.
func (s *Storage) LoadCheckpoint(r io.Reader) error {
	capacity, err := readUint64(r)
	if err != nil {
		return err
	}
	unitSize, err := readUint64(r)
	if err != nil {
		return err
	}

	if capacity != s.capacity {
		return fmt.Errorf(
			"mem: storage %q capacity mismatch: checkpoint %d, rebuilt %d",
			s.name, capacity, s.capacity)
	}
	if unitSize != s.unitSize {
		return fmt.Errorf(
			"mem: storage %q unit size mismatch: checkpoint %d, rebuilt %d",
			s.name, unitSize, s.unitSize)
	}

	numUnits, err := readUint64(r)
	if err != nil {
		return err
	}

	data := make(map[uint64]*storageUnit, numUnits)
	for i := uint64(0); i < numUnits; i++ {
		addr, err := readUint64(r)
		if err != nil {
			return err
		}
		unit := newStorageUnit(s.unitSize)
		if _, err := io.ReadFull(r, unit.data); err != nil {
			return err
		}
		data[addr] = unit
	}

	s.Lock()
	s.data = data
	s.Unlock()

	return nil
}

func writeUint64(w io.Writer, v uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	_, err := w.Write(buf[:])
	return err
}

func readUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}
