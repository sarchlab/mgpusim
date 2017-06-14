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

	It("should read SGPRs", func() {
		wf.SRegFile[0] = 1

		buf := make([]byte, 4)
		regInterface.ReadReg(wf, 0, insts.SReg(0), buf)

		Expect(buf[0]).To(Equal(byte(1)))
	})

	It("should write SGPRs", func() {
		buf := make([]byte, 4)
		buf[0] = 1
		regInterface.WriteReg(wf, 0, insts.SReg(0), buf)

		Expect(wf.SRegFile[0]).To(Equal(byte(1)))
	})

	It("should read VGPRs", func() {
		wf.VRegFile[1028] = 1

		buf := make([]byte, 4)
		regInterface.ReadReg(wf, 1, insts.VReg(1), buf)

		Expect(buf[0]).To(Equal(byte(1)))
	})

	It("should write VGPRs", func() {
		buf := make([]byte, 4)
		buf[0] = 1

		regInterface.WriteReg(wf, 1, insts.VReg(1), buf)

		Expect(wf.VRegFile[1028]).To(Equal(byte(1)))
	})
})
