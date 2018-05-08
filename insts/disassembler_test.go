package insts_test

import (
	"bufio"
	"bytes"
	"debug/elf"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/yaotsu/gcn3/insts"
)

func TestDisassembler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Disassembler")
}

var _ = Describe("Disassembler", func() {
	It("should disassemble kernel 1", func() {
		var buf bytes.Buffer

		elfFile, err := elf.Open("../samples/firsim/kernels.hsaco")
		defer elfFile.Close()
		Expect(err).To(BeNil())

		targetFile, err := os.Open("../samples/firsim/kernels.isa")
		Expect(err).To(BeNil())
		defer targetFile.Close()

		disasm := insts.NewDisassembler()

		disasm.Disassemble(elfFile, &buf)

		resultScanner := bufio.NewScanner(&buf)
		targetScanner := bufio.NewScanner(targetFile)
		for targetScanner.Scan() {
			Expect(resultScanner.Scan()).To(Equal(true))
			Expect(resultScanner.Text()).To(Equal(targetScanner.Text()))
		}

	})
})
