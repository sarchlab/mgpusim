package gcn3

import (
	"log"
	"reflect"
	"sync"

	"github.com/onsi/gomega"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A ComputeUnit is where the GPU kernel is executed, in the unit of work group.
type ComputeUnit interface {
	core.Component

	WriteReg(reg *disasm.Reg, wiFlatID int, data []byte)
	ReadReg(reg *disasm.Reg, wiFlatID int, byteSize int) []byte

	WriteMem(address uint64, data []byte) *core.Error
	ReadMem(address uint64, size int) *core.Error
	ReadInstMem(addr uint64, size int, info interface{},
		now core.VTimeInSec) *core.Error
}

type regWrite struct {
	reg      *disasm.Reg
	wiFlatID int
	data     []byte
}

type regRead struct {
	reg      *disasm.Reg
	wiFlatID int
	byteSize int
	data     []byte
}

// MockComputeUnit is a ComputeUnit that is designed for help with testing
type MockComputeUnit struct {
	*core.BasicComponent
	lock             sync.Mutex
	expectedRegWrite []regWrite
	expectedRegRead  []regRead
}

// NewMockComputeUnit returns a new MockComputeUnit
func NewMockComputeUnit(name string) *MockComputeUnit {
	cu := new(MockComputeUnit)
	cu.BasicComponent = core.NewBasicComponent(name)
	cu.expectedRegWrite = make([]regWrite, 0)
	cu.expectedRegRead = make([]regRead, 0)
	return cu
}

// Receive function of a MockComputeUnit dost not do anything.
func (cu *MockComputeUnit) Receive(req core.Request) *core.Error {
	return nil
}

// Handle function of a MockComputeUnit does not do anything
func (cu *MockComputeUnit) Handle(evt core.Event) error {
	return nil
}

// ExpectRegWrite registers a write register action that is expected to happen
func (cu *MockComputeUnit) ExpectRegWrite(
	reg *disasm.Reg,
	wiFlatID int,
	data []byte,
) {
	cu.expectedRegWrite = append(cu.expectedRegWrite,
		regWrite{reg, wiFlatID, data})
}

// ExpectRegRead registers a write register action that is expected to happen
func (cu *MockComputeUnit) ExpectRegRead(
	reg *disasm.Reg,
	wiFlatID int,
	byteSize int,
	data []byte,
) {
	cu.expectedRegRead = append(cu.expectedRegRead,
		regRead{reg, wiFlatID, byteSize, data})
}

// WriteReg function of a MockComputeUnit checks if a write action is expected
func (cu *MockComputeUnit) WriteReg(reg *disasm.Reg, wiFlatID int,
	data []byte,
) {

	cu.lock.Lock()
	defer cu.lock.Unlock()

	for i, regWrite := range cu.expectedRegWrite {
		if regWrite.reg == reg &&
			regWrite.wiFlatID == wiFlatID &&
			reflect.DeepEqual(regWrite.data, data) {
			cu.expectedRegWrite = append(cu.expectedRegWrite[:i],
				cu.expectedRegWrite[i+1:]...)
			return
		}
	}

	log.Panicf("Writing to register %s is not expected", reg.Name)
}

// ReadReg function of a MockComputeUnit checks if a read action is expected
func (cu *MockComputeUnit) ReadReg(reg *disasm.Reg, wiFlatID int,
	byteSize int,
) []byte {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	for i, regRead := range cu.expectedRegRead {
		if regRead.reg == reg &&
			regRead.wiFlatID == wiFlatID &&
			regRead.byteSize == byteSize {
			cu.expectedRegRead = append(cu.expectedRegRead[:i],
				cu.expectedRegRead[i+1:]...)
			return regRead.data
		}
	}

	log.Panicf("Reading register %s is not expected", reg.Name)
	return nil
}

// AllExpectedAccessed expects that all the expected read an write actions
// are actually performed
func (cu *MockComputeUnit) AllExpectedAccessed() {
	gomega.Expect(cu.expectedRegRead).To(gomega.BeEmpty())
	gomega.Expect(cu.expectedRegWrite).To(gomega.BeEmpty())
}

// WriteMem is not implmented
func (cu *MockComputeUnit) WriteMem(address uint64, data []byte) *core.Error {
	return nil
}

// ReadMem is not implmented
func (cu *MockComputeUnit) ReadMem(address uint64, size int) *core.Error {
	return nil
}

// ReadInstMem is not implemented
func (cu *MockComputeUnit) ReadInstMem(addr uint64, size int,
	info interface{}, now core.VTimeInSec,
) *core.Error {
	return nil
}
