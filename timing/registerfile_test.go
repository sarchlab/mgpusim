package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("Simple Register File", func() {

	var (
		registerFile *SimpleRegisterFile
	)

	BeforeEach(func() {
		registerFile = NewSimpleRegisterFile(26152, 0)
	})

	It("should write scalar registers", func() {
		regWrite := new(RegisterAccess)
		regWrite.Reg = insts.SReg(0)
		regWrite.LaneID = 0
		regWrite.WaveOffset = 0
		regWrite.Data = insts.Uint32ToBytes(15)

		registerFile.Write(regWrite)

		data, _ := registerFile.storage.Read(0, 4)
		Expect(regWrite.OK).To(BeTrue())
		Expect(insts.BytesToUint32(data)).To(Equal(uint32(15)))
	})

	It("should read scalar registers", func() {
		registerFile.storage.Write(104, insts.Uint32ToBytes(16))

		regRead := new(RegisterAccess)
		regRead.Reg = insts.SReg(1)
		regRead.LaneID = 1
		regRead.WaveOffset = 100
		regRead.RegCount = 1

		registerFile.Read(regRead)

		Expect(regRead.OK).To(BeTrue())
		Expect(insts.BytesToUint32(regRead.Data)).To(Equal(uint32(16)))
	})

	It("should write vector registers", func() {
		registerFile.ByteSizePerLane = 1024

		regWrite := new(RegisterAccess)
		regWrite.Reg = insts.VReg(10)
		regWrite.LaneID = 5
		regWrite.WaveOffset = 100
		regWrite.Data = insts.Uint32ToBytes(15)

		registerFile.Write(regWrite)

		data, _ := registerFile.storage.Read(100+10*4+1024*5, 4)
		Expect(regWrite.OK).To(BeTrue())
		Expect(insts.BytesToUint32(data)).To(Equal(uint32(15)))
	})

})
