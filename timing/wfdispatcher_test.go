package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

var _ = Describe("WfDispatcher", func() {
	var (
		engine       *core.MockEngine
		cu           *ComputeUnit
		wfDispatcher *WfDispatcherImpl
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		cu = NewComputeUnit("cu", engine)
		cu.Freq = 1
		wfDispatcher = NewWfDispatcher(cu)
	})

	It("should dispatch wavefront", func() {
		rawWf := kernels.NewWavefront()
		wfDispatchInfo := &WfDispatchInfo{rawWf, 1, 16, 8, 512}
		cu.WfToDispatch[rawWf] = wfDispatchInfo

		co := insts.NewHsaCo()
		co.KernelCodeEntryByteOffset = 256
		packet := new(kernels.HsaKernelDispatchPacket)
		packet.KernelObject = 65536

		wf := NewWavefront(rawWf)
		wf.CodeObject = co
		wf.Packet = packet
		req := gcn3.NewDispatchWfReq(nil, cu, 10, nil)
		wfDispatcher.DispatchWf(wf, req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(wf.SIMDID).To(Equal(1))
		Expect(wf.VRegOffset).To(Equal(16))
		Expect(wf.SRegOffset).To(Equal(8))
		Expect(wf.LDSOffset).To(Equal(512))
		Expect(wf.PC).To(Equal(uint64(65536 + 256)))
	})
})
