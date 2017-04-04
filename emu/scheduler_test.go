package emu_test

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
)

type MockDecoder struct {
	Buf          []byte
	InstToReturn *insts.Inst
}

func (d *MockDecoder) Decode(buf []byte) (*insts.Inst, error) {
	d.Buf = buf
	return d.InstToReturn, nil
}

type MockInstWorker struct {
	Wf  *emu.WfScheduleInfo
	Now core.VTimeInSec
}

func (w *MockInstWorker) Run(wf *emu.WfScheduleInfo, now core.VTimeInSec) error {
	w.Wf = wf
	w.Now = now
	return nil
}

var _ = ginkgo.Describe("Schedule", func() {
	var (
		scheduler  *emu.Scheduler
		cu         *gcn3.MockComputeUnit
		decoder    *MockDecoder
		instWorker *MockInstWorker
	)

	ginkgo.BeforeEach(func() {
		scheduler = emu.NewScheduler()
		decoder = new(MockDecoder)
		instWorker = new(MockInstWorker)
		cu = gcn3.NewMockComputeUnit("cu")
		scheduler.CU = cu
		scheduler.Decoder = decoder
		scheduler.InstWorker = instWorker
	})

	ginkgo.It("should schedule fetch", func() {
		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)

		cu.ExpectRegRead(insts.Regs[insts.Pc], 0, 8,
			insts.Uint64ToBytes(4000))
		cu.ExpectReadInstMem(4000, 8, nil, 0)

		scheduler.Schedule(0)

		cu.AllExpectedAccessed()
		gomega.Expect(scheduler.Wfs[0].State).To(gomega.Equal(emu.Fetching))
	})

	ginkgo.It("should mark fetched", func() {
		data := make([]byte, 8)
		wf := new(emu.WfScheduleInfo)

		scheduler.Fetched(wf, data)

		gomega.Expect(wf.State).To(gomega.Equal(emu.Fetched))
		gomega.Expect(wf.InstBuf).To(gomega.Equal(data))
	})

	ginkgo.It("should decode", func() {
		inst := insts.NewInstruction()
		decoder.InstToReturn = inst

		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)
		scheduler.Wfs[0].State = emu.Fetched

		scheduler.Schedule(0)

		gomega.Expect(scheduler.Wfs[0].State).To(gomega.Equal(emu.Decoded))
	})

	ginkgo.It("should issue", func() {
		inst := insts.NewInstruction()

		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)
		scheduler.Wfs[0].State = emu.Decoded
		scheduler.Wfs[0].Inst = inst

		scheduler.Schedule(0)

		gomega.Expect(scheduler.Wfs[0].State).To(gomega.Equal(emu.Running))
		gomega.Expect(instWorker.Wf).To(gomega.BeIdenticalTo(scheduler.Wfs[0]))
		gomega.Expect(instWorker.Now).To(gomega.BeNumerically("~", 0, 1e-9))
	})
})
