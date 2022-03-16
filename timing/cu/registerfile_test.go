package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v2/insts"
)

var _ = Describe("Simple Register File", func() {

	var (
		registerFile *SimpleRegisterFile
	)

	BeforeEach(func() {
		registerFile = NewSimpleRegisterFile(26152, 0)
	})

	It("should write scalar registers", func() {
		regWrite := RegisterAccess{}
		regWrite.Reg = insts.SReg(0)
		regWrite.LaneID = 0
		regWrite.WaveOffset = 0
		regWrite.Data = insts.Uint32ToBytes(15)

		registerFile.Write(regWrite)

		data := registerFile.storage[0:4]
		//Expect(regWrite.OK).To(BeTrue())
		Expect(insts.BytesToUint32(data)).To(Equal(uint32(15)))
	})

	It("should read scalar registers", func() {
		copy(registerFile.storage[104:108], insts.Uint32ToBytes(16))

		regRead := RegisterAccess{}
		regRead.Reg = insts.SReg(1)
		regRead.LaneID = 1
		regRead.WaveOffset = 100
		regRead.RegCount = 1
		regRead.Data = make([]byte, regRead.RegCount*4)

		registerFile.Read(regRead)

		//Expect(regRead.OK).To(BeTrue())
		Expect(insts.BytesToUint32(regRead.Data)).To(Equal(uint32(16)))
	})

	It("should write vector registers", func() {
		registerFile.ByteSizePerLane = 1024

		regWrite := RegisterAccess{}
		regWrite.Reg = insts.VReg(10)
		regWrite.LaneID = 5
		regWrite.WaveOffset = 100
		regWrite.Data = insts.Uint32ToBytes(15)

		registerFile.Write(regWrite)

		offset := 100 + 10*4 + 1024*5
		data := registerFile.storage[offset : offset+4]
		//Expect(regWrite.OK).To(BeTrue())
		Expect(insts.BytesToUint32(data)).To(Equal(uint32(15)))
	})

})
