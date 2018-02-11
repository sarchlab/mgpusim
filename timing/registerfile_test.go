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
		registerFile = NewSimpleRegisterFile(26152)
	})

	It("should read and write scalar registers", func() {
		writeOK := registerFile.Write(
			insts.SReg(0),
			0,
			0,
			insts.Uint32ToBytes(15),
			10,
		)

		readOK, output := registerFile.Read(
			insts.SReg(0),
			0,
			0,
			10,
		)

		Expect(writeOK).To(BeTrue())
		Expect(readOK).To(BeTrue())
	})

})
