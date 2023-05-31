package cache

import (
	"log"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

// MSHREntry is an entry in MSHR
type MSHREntry struct {
	PID       vm.PID
	Address   uint64
	Requests  []interface{}
	Block     *Block
	ReadReq   *mem.ReadReq
	DataReady *mem.DataReadyRsp
	Data      []byte
}

// NewMSHREntry returns a new MSHR entry object
func NewMSHREntry() *MSHREntry {
	e := new(MSHREntry)
	e.Requests = make([]interface{}, 0)
	return e
}

// MSHR is an interface that controls MSHR entries
type MSHR interface {
	Query(pid vm.PID, addr uint64) *MSHREntry
	Add(pid vm.PID, addr uint64) *MSHREntry
	Remove(pid vm.PID, addr uint64) *MSHREntry
	AllEntries() []*MSHREntry
	IsFull() bool
	Reset()
}

// NewMSHR returns a new MSHR object
func NewMSHR(capacity int) MSHR {
	m := new(mshrImpl)
	m.capacity = capacity
	return m
}

type mshrImpl struct {
	*sim.ComponentBase

	capacity int
	entries  []*MSHREntry
}

func (m *mshrImpl) Add(pid vm.PID, addr uint64) *MSHREntry {
	for _, e := range m.entries {
		if e.PID == pid && e.Address == addr {
			panic("entry already in mshr")
		}
	}

	if len(m.entries) >= m.capacity {
		log.Panic("MSHR is full")
	}

	entry := NewMSHREntry()
	entry.PID = pid
	entry.Address = addr
	m.entries = append(m.entries, entry)
	return entry
}

func (m *mshrImpl) Query(pid vm.PID, addr uint64) *MSHREntry {
	for _, e := range m.entries {
		if e.PID == pid && e.Address == addr {
			return e
		}
	}
	return nil
}

func (m *mshrImpl) Remove(pid vm.PID, addr uint64) *MSHREntry {
	for i, e := range m.entries {
		if e.PID == pid && e.Address == addr {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return e
		}
	}
	panic("trying to remove an non-exist entry")
}

// AllEntries returns all the MSHREntries that are currently in the MSHR
func (m *mshrImpl) AllEntries() []*MSHREntry {
	return m.entries
}

// IsFull returns true if no more MSHR entries can be added
func (m *mshrImpl) IsFull() bool {
	if len(m.entries) >= m.capacity {
		return true
	}
	return false
}

func (m *mshrImpl) Reset() {
	m.entries = nil
}
