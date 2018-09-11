package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

var _ = Describe("WfDispatcher", func() {
	var (
		engine       *akita.MockEngine
		cu           *ComputeUnit
		wfDispatcher *WfDispatcherImpl
	)

	BeforeEach(func() {
		engine = akita.NewMockEngine()
		cu = NewComputeUnit("cu", engine)
		cu.Freq = 1

		sRegFile := NewSimpleRegisterFile(uint64(3200*4), 0)
		cu.SRegFile = sRegFile

		for i := 0; i < 4; i++ {
			vRegFile := NewSimpleRegisterFile(uint64(16384*4), 1024)
			cu.VRegFile = append(cu.VRegFile, vRegFile)
		}

		wfDispatcher = NewWfDispatcher(cu)
	})

	It("should dispatch wavefront", func() {
		rawWf := kernels.NewWavefront()
		rawWG := kernels.NewWorkGroup()
		rawWf.WG = rawWG
		rawWG.SizeX = 256
		rawWG.SizeY = 1
		rawWG.SizeZ = 1
		wfDispatchInfo := &WfDispatchInfo{rawWf, 1, 16, 8, 512}
		cu.WfToDispatch[rawWf] = wfDispatchInfo

		co := insts.NewHsaCo()
		co.KernelCodeEntryByteOffset = 256
		packet := new(kernels.HsaKernelDispatchPacket)
		packet.KernelObject = 65536

		wf := NewWavefront(rawWf)
		wg := NewWorkGroup(rawWG, nil)
		wf.WG = wg
		wf.CodeObject = co
		wf.Packet = packet
		//req := gcn3.NewDispatchWfReq(nil, cu.ToACE, 10, nil)
		wfDispatcher.DispatchWf(10, wf)

		//Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(wf.SIMDID).To(Equal(1))
		Expect(wf.VRegOffset).To(Equal(16))
		Expect(wf.SRegOffset).To(Equal(8))
		Expect(wf.LDSOffset).To(Equal(512))
		Expect(wf.PC).To(Equal(uint64(65536 + 256)))
	})
})
