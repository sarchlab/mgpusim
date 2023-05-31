package tlb

import (
	"log"

	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

type mshrEntry struct {
	pid         vm.PID
	vAddr       uint64
	Requests    []*vm.TranslationReq
	reqToBottom *vm.TranslationReq
	page        vm.Page
}

// newMSHREntry returns a new MSHR entry object
func newMSHREntry() *mshrEntry {
	e := new(mshrEntry)
	return e
}

// mshr is an interface that controls MSHR entries
type mshr interface {
	Query(pid vm.PID, addr uint64) *mshrEntry
	Add(pid vm.PID, addr uint64) *mshrEntry
	Remove(pid vm.PID, addr uint64) *mshrEntry
	AllEntries() []*mshrEntry
	IsFull() bool
	Reset()
	GetEntry(pid vm.PID, vAddr uint64) *mshrEntry
	IsEntryPresent(pid vm.PID, vAddr uint64) bool
}

type mshrImpl struct {
	capacity int
	entries  []*mshrEntry
}

// newMSHR returns a new mshr object
func newMSHR(capacity int) mshr {
	m := new(mshrImpl)
	m.capacity = capacity
	return m
}

func (m *mshrImpl) Add(pid vm.PID, vAddr uint64) *mshrEntry {
	for _, e := range m.entries {
		if e.pid == pid && e.vAddr == vAddr {
			panic("entry already in mshr")
		}
	}

	if len(m.entries) >= m.capacity {
		log.Panic("MSHR is full")
	}

	entry := newMSHREntry()
	entry.pid = pid
	entry.vAddr = vAddr
	m.entries = append(m.entries, entry)
	return entry
}

func (m *mshrImpl) Query(pid vm.PID, vAddr uint64) *mshrEntry {
	for _, e := range m.entries {
		if e.pid == pid && e.vAddr == vAddr {
			return e
		}
	}
	return nil
}

func (m *mshrImpl) Remove(pid vm.PID, vAddr uint64) *mshrEntry {
	for i, e := range m.entries {
		if e.pid == pid && e.vAddr == vAddr {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return e
		}
	}
	panic("trying to remove an non-exist entry")
}

func (m *mshrImpl) AllEntries() []*mshrEntry {
	return m.entries
}

func (m *mshrImpl) IsFull() bool {
	return len(m.entries) >= m.capacity
}

func (m *mshrImpl) Reset() {
	m.entries = nil
}

func (m *mshrImpl) GetEntry(pid vm.PID, vAddr uint64) *mshrEntry {
	for _, e := range m.entries {
		if e.pid == pid && e.vAddr == vAddr {
			return e
		}
	}
	return nil
}

func (m *mshrImpl) IsEntryPresent(pid vm.PID, vAddr uint64) bool {
	for _, e := range m.entries {
		if e.pid == pid && e.vAddr == vAddr {
			return true
		}
	}
	return false
}
