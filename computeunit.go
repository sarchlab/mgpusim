package gcn3

import (
	"github.com/onsi/gomega"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A ComputeUnit is where the GPU kernel is executed, in the unit of work group.
type ComputeUnit interface {
	core.Component

	WriteReg(reg *disasm.Reg, wiFlatID int, data []byte)
	ReadReg(reg *disasm.Reg, wiFlatID int, byteSize int) []byte
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
	data []byte) {

	gomega.Expect(cu.expectedRegWrite).NotTo(gomega.BeEmpty())

	head := cu.expectedRegWrite[0]
	gomega.Expect(head.reg).To(gomega.Equal(reg))
	gomega.Expect(head.wiFlatID).To(gomega.Equal(wiFlatID))
	gomega.Expect(head.data).To(gomega.ConsistOf(data))
	cu.expectedRegWrite = cu.expectedRegWrite[1:]
}

// ReadReg function of a MockComputeUnit checks if a read action is expected
func (cu *MockComputeUnit) ReadReg(reg *disasm.Reg, wiFlatID int,
	byteSize int) []byte {

	gomega.Expect(cu.expectedRegRead).NotTo(gomega.BeEmpty())
	head := cu.expectedRegRead[0]
	gomega.Expect(head.reg).To(gomega.Equal(reg))
	gomega.Expect(head.wiFlatID).To(gomega.Equal(wiFlatID))
	gomega.Expect(head.byteSize).To(gomega.Equal(byteSize))
	cu.expectedRegRead = cu.expectedRegRead[1:]
	return head.data
}

// AllExpectedAccessed expects that all the expected read an write actions
// are actually performed
func (cu *MockComputeUnit) AllExpectedAccessed() {
	gomega.Expect(cu.expectedRegRead).To(gomega.BeEmpty())
	gomega.Expect(cu.expectedRegWrite).To(gomega.BeEmpty())
}

// Receive function of a MockComputeUnit dost not do anything.
func (cu *MockComputeUnit) Receive(req core.Request) *core.Error {
	return nil
}

// Handle function of a MockComputeUnit does not do anything
func (cu *MockComputeUnit) Handle(evt core.Event) error {
	return nil
}
