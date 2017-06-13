package emu

import (
	"log"

	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3/insts"
)

type regToAccess struct {
	Wf     interface{}
	laneID int
	Reg    *insts.Reg
	Data   []byte
}

type mockRegInterface struct {
	regToRead  []*regToAccess
	regToWrite []*regToAccess
}

func newMockRegInterface() *mockRegInterface {
	i := new(mockRegInterface)
	i.regToRead = make([]*regToAccess, 0)
	i.regToWrite = make([]*regToAccess, 0)
	return i
}

func (i *mockRegInterface) RegToRead(
	wf interface{},
	laneID int,
	reg *insts.Reg,
	data []byte,
) {
	i.regToRead = append(i.regToRead, &regToAccess{wf, laneID, reg, data})
}

func (i *mockRegInterface) RegToWrite(
	wf interface{},
	laneID int,
	reg *insts.Reg,
	data []byte,
) {
	i.regToWrite = append(i.regToWrite, &regToAccess{wf, laneID, reg, data})
}

func (i *mockRegInterface) ReadReg(
	wf interface{},
	laneID int,
	reg *insts.Reg,
	writeTo []byte,
) {
	for index, regToAccess := range i.regToRead {
		if regToAccess.Wf == wf &&
			regToAccess.laneID == laneID &&
			reg == regToAccess.Reg &&
			len(writeTo) == len(regToAccess.Data) {

			i.regToRead[index] = i.regToRead[len(i.regToRead)-1]
			i.regToRead = i.regToRead[:len(i.regToRead)-1]

			copy(writeTo, regToAccess.Data)

			return
		}
	}

	log.Panicf("Register %s reading not expected", reg.Name)
}

func (i *mockRegInterface) WriteReg(
	wf interface{},
	laneID int,
	reg *insts.Reg,
	writeFrom []byte,
) {
	for index, regToAccess := range i.regToRead {
		if regToAccess.Wf == wf &&
			regToAccess.laneID == laneID &&
			reg == regToAccess.Reg &&
			len(writeFrom) == len(regToAccess.Data) {

			writeSameData := true
			for j := range writeFrom {
				if writeFrom[j] != regToAccess.Data[j] {
					writeSameData = false
					break
				}
			}

			if !writeSameData {
				break
			}

			i.regToWrite[index] = i.regToWrite[len(i.regToWrite)-1]
			i.regToWrite = i.regToWrite[:len(i.regToWrite)-1]

			return
		}
	}

	log.Panicf("Register %s writing not expected", reg.Name)
}

func (i *mockRegInterface) AllExpectedAccessed() bool {
	return len(i.regToRead) == 0 && len(i.regToWrite) == 0
}

var _ = Describe("ScratchpadPreparer", func() {

	var (
		regInterface *mockRegInterface
		sp           *ScratchpadPreparerImpl
		wf           *Wavefront
	)

	BeforeEach(func() {
		regInterface = newMockRegInterface()
		sp = NewScratchpadPreparerImpl(regInterface)
		wf = NewWavefront(nil)
	})

	It("should prepare for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.Sop2
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 1)
		wf.inst = inst

		regInterface.RegToRead(wf, 0, insts.Regs[insts.S0], []byte{1, 2, 3, 4})
		regInterface.RegToRead(wf, 0, insts.Regs[insts.Scc], []byte{1})

		sp.Prepare(wf, wf)
	})
})
