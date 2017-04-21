package cu_test

import (
	. "github.com/onsi/ginkgo"
	"gitlab.com/yaotsu/gcn3/timing/cu"
	"gitlab.com/yaotsu/mem"
)

var _ = Describe("RegCtrl", func() {

	var (
		regCtrl *regCtrl
	)

	BeforeEach(func() {
		regCtrl = cu.NewRegCtrl("ScalarReg", 8*mem.KB)
	})

	Context("when processing ReqReadReq", func() {
		It("should return the register value", func() {
		})
	})
})
