package disasm_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/disasm"
)

var _ = Describe("Register", func() {
	It("should get correct v register", func() {
		Expect(disasm.VReg(0)).To(BeIdenticalTo(disasm.Regs[disasm.V0]))
		Expect(disasm.VReg(5)).To(BeIdenticalTo(disasm.Regs[disasm.V5]))
	})

	It("should get correct s register", func() {
		Expect(disasm.SReg(0)).To(BeIdenticalTo(disasm.Regs[disasm.S0]))
		Expect(disasm.SReg(5)).To(BeIdenticalTo(disasm.Regs[disasm.S5]))
	})

})
