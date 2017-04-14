package gcn3

import (
	"log"
	"reflect"
	"sync"

	"github.com/onsi/gomega"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A ComputeUnit is where the GPU kernel is executed, in the unit of work group.
type ComputeUnit interface {
	core.Component

	WriteReg(reg *insts.Reg, wiFlatID int, data []byte)
	ReadReg(reg *insts.Reg, wiFlatID int, byteSize int) []byte

	WriteMem(
		address uint64, data []byte,
		info interface{},
		now core.VTimeInSec) (*mem.AccessReq, *core.Error)
	ReadMem(address uint64, size int,
		info interface{},
		now core.VTimeInSec) (*mem.AccessReq, *core.Error)
	ReadInstMem(
		addr uint64, size int,
		info interface{},
		now core.VTimeInSec) (*mem.AccessReq, *core.Error)
}

type regWrite struct {
	reg      *insts.Reg
	wiFlatID int
	data     []byte
}

type regRead struct {
	reg      *insts.Reg
	wiFlatID int
	byteSize int
	data     []byte
}

type memRead struct {
	addr uint64
	size int
	info interface{}
	now  core.VTimeInSec
	req  *mem.AccessReq
	err  *core.Error
}

type memWrite struct {
	addr uint64
	data []byte
	info interface{}
	now  core.VTimeInSec
	req  *mem.AccessReq
	err  *core.Error
}

// MockComputeUnit is a ComputeUnit that is designed for help with testing
type MockComputeUnit struct {
	*core.BasicComponent
	lock                sync.Mutex
	expectedRegWrite    []regWrite
	expectedRegRead     []regRead
	expectedInstMemRead []memRead
	expectedMemRead     []memRead
}

// NewMockComputeUnit returns a new MockComputeUnit
func NewMockComputeUnit(name string) *MockComputeUnit {
	cu := new(MockComputeUnit)
	cu.BasicComponent = core.NewBasicComponent(name)
	cu.expectedRegWrite = make([]regWrite, 0)
	cu.expectedRegRead = make([]regRead, 0)
	cu.expectedInstMemRead = make([]memRead, 0)
	cu.expectedMemRead = make([]memRead, 0)
	return cu
}

// Recv function of a MockComputeUnit dost not do anything.
func (cu *MockComputeUnit) Recv(req core.Req) *core.Error {
	return nil
}

// Handle function of a MockComputeUnit does not do anything
func (cu *MockComputeUnit) Handle(evt core.Event) error {
	return nil
}

// ExpectRegWrite registers a write register action that is expected to happen
func (cu *MockComputeUnit) ExpectRegWrite(
	reg *insts.Reg,
	wiFlatID int,
	data []byte,
) {
	cu.expectedRegWrite = append(cu.expectedRegWrite,
		regWrite{reg, wiFlatID, data})
}

// ExpectRegRead registers a write register action that is expected to happen
func (cu *MockComputeUnit) ExpectRegRead(
	reg *insts.Reg,
	wiFlatID int,
	byteSize int,
	data []byte,
) {
	cu.expectedRegRead = append(cu.expectedRegRead,
		regRead{reg, wiFlatID, byteSize, data})
}

// WriteReg function of a MockComputeUnit checks if a write action is expected
func (cu *MockComputeUnit) WriteReg(reg *insts.Reg, wiFlatID int,
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
func (cu *MockComputeUnit) ReadReg(reg *insts.Reg, wiFlatID int,
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
	gomega.Expect(cu.expectedInstMemRead).To(gomega.BeEmpty())
	gomega.Expect(cu.expectedMemRead).To(gomega.BeEmpty())
}

// WriteMem is not implmented
func (cu *MockComputeUnit) WriteMem(
	address uint64,
	data []byte,
	info interface{},
	now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	return nil, nil
}

// ReadMem is not implmented
func (cu *MockComputeUnit) ReadMem(
	address uint64,
	size int,
	info interface{},
	now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()
	for i, r := range cu.expectedMemRead {
		if r.addr == address {
			cu.expectedMemRead = append(cu.expectedMemRead[:i],
				cu.expectedMemRead[i+1:]...)
			return r.req, r.err
		}
	}
	log.Panicf("Memory Read to address %d is not expected", address)
	return nil, nil
}

// ExpectReadMem registers an ReadMem action that is to be called in the future
func (cu *MockComputeUnit) ExpectReadMem(
	address uint64, size int,
	info interface{}, now core.VTimeInSec,
	req *mem.AccessReq, err *core.Error,
) {
	r := memRead{address, size, info, now, req, err}
	cu.expectedMemRead = append(cu.expectedMemRead, r)
}

// ExpectReadInstMem registers an ReadInstMem action that is to happen in the
// future
func (cu *MockComputeUnit) ExpectReadInstMem(
	addr uint64, size int,
	info interface{}, now core.VTimeInSec,
	req *mem.AccessReq, err *core.Error,
) {
	r := memRead{addr, size, info, now, req, err}
	cu.expectedInstMemRead = append(cu.expectedInstMemRead, r)
}

// ReadInstMem is not implemented
func (cu *MockComputeUnit) ReadInstMem(
	addr uint64, size int,
	info interface{}, now core.VTimeInSec,
) (*mem.AccessReq, *core.Error) {
	cu.lock.Lock()
	defer cu.lock.Unlock()

	gomega.Expect(cu.expectedInstMemRead).NotTo(gomega.BeEmpty())
	gomega.Expect(addr).To(gomega.Equal(cu.expectedInstMemRead[0].addr))
	gomega.Expect(size).To(gomega.Equal(cu.expectedInstMemRead[0].size))
	gomega.Expect(now).To(gomega.Equal(cu.expectedInstMemRead[0].now))

	cu.expectedInstMemRead = cu.expectedInstMemRead[1:]

	return nil, nil
}
