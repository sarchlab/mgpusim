package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/kernels"
	"gitlab.com/akita/mgpusim/v3/protocol"
	"gitlab.com/akita/mgpusim/v3/timing/wavefront"
)

var _ = Describe("WfDispatcher", func() {
	var (
		cu           *ComputeUnit
		wfDispatcher *WfDispatcherImpl
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
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
		wfDispatchInfo := protocol.WfDispatchLocation{
			Wavefront:  rawWf,
			SIMDID:     1,
			VGPROffset: 16,
			SGPROffset: 8,
			LDSOffset:  512,
		}

		co := insts.NewHsaCo()
		co.KernelCodeEntryByteOffset = 256
		packet := new(kernels.HsaKernelDispatchPacket)
		packet.KernelObject = 65536

		wf := wavefront.NewWavefront(rawWf)
		wg := wavefront.NewWorkGroup(rawWG, nil)
		wf.WG = wg
		wf.CodeObject = co
		wf.Packet = packet
		//req := mgpusim.NewDispatchWfReq(nil, cu.ToACE, 10, nil)
		wfDispatcher.DispatchWf(10, wf, wfDispatchInfo)

		//Expect(len(engine.ScheduledEvent)).To(Equal(1))
		Expect(wf.SIMDID).To(Equal(1))
		Expect(wf.VRegOffset).To(Equal(16))
		Expect(wf.SRegOffset).To(Equal(8))
		Expect(wf.LDSOffset).To(Equal(512))
		Expect(wf.PC).To(Equal(uint64(65536 + 256)))
	})
})
