package insts_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
)

func TestDisassembler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Disassembler")
}

var _ = Describe("Disassembler", func() {

	var (
		disassembler *insts.Disassembler
	)

	BeforeEach(func() {
		disassembler = insts.NewDisassembler()
	})

	It("should decode BF8C0F70", func() {
		buf := []byte{0x80, 0x0f, 0x8c, 0xbf}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("s_waitcnt vmcnt(0)"))
	})

	It("should decode D81A0004 00000210", func() {
		buf := []byte{0x04, 0x00, 0x1A, 0xd8, 0x10, 0x02, 0x00, 0x00}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("ds_write_b32 v16, v2 offset:4"))
	})

	It("should decode D86C0008 01000010", func() {
		buf := []byte{0x08, 0x00, 0x6c, 0xd8, 0x10, 0x00, 0x00, 0x01}

		inst, err := disassembler.Decode(buf)

		Expect(err).To(BeNil())
		Expect(inst.String(nil)).To(Equal("ds_read_b32 v1, v16 offset:8"))

	})

	//It("should disassemble kernel 1", func() {
	//var buf bytes.Buffer

	//elfFile, err := elf.Open("../benchmarks/heteromark/fir/kernels.hsaco")
	//defer elfFile.Close()
	//Expect(err).To(BeNil())

	//targetFile, err := os.Open("../benchmarks/heteromark/fir/kernels.disasm")
	//Expect(err).To(BeNil())
	//defer targetFile.Close()

	//disasm := insts.NewDisassembler()

	//disasm.Disassemble(elfFile, "kernels.hsaco", &buf)

	//resultScanner := bufio.NewScanner(&buf)
	//targetScanner := bufio.NewScanner(targetFile)
	//for targetScanner.Scan() {
	//Expect(resultScanner.Scan()).To(Equal(true))
	//Expect(resultScanner.Text()).To(Equal(targetScanner.Text()))
	//}

	//})
})
