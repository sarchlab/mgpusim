package emu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("RegInterface", func() {
	var (
		regInterface *RegInterfaceImpl
		wf           *Wavefront
	)

	BeforeEach(func() {
		regInterface = new(RegInterfaceImpl)
		wf = NewWavefront(nil)
	})

	It("should read register", func() {
		wf.SRegFile[0] = 1

		buf := make([]byte, 4)
		regInterface.ReadReg(wf, insts.SReg(0), buf)

		Expect(buf[0]).To(Equal(byte(1)))
	})
})
