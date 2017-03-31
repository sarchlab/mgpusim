package emu_test

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

type MockDecoder struct {
	Buf          []byte
	InstToReturn *disasm.Instruction
}

func (d *MockDecoder) Decode(buf []byte) (*disasm.Instruction, error) {
	d.Buf = buf
	return d.InstToReturn, nil
}

var _ = ginkgo.Describe("Schedule", func() {
	var (
		scheduler *emu.Scheduler
		cu        *gcn3.MockComputeUnit
		decoder   *MockDecoder
	)

	ginkgo.BeforeEach(func() {
		scheduler = emu.NewScheduler()
		decoder = new(MockDecoder)
		cu = gcn3.NewMockComputeUnit("cu")
		scheduler.CU = cu
		scheduler.Decoder = decoder
	})

	ginkgo.It("should schedule fetch", func() {
		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)

		cu.ExpectRegRead(disasm.Regs[disasm.Pc], 0, 8,
			disasm.Uint64ToBytes(4000))
		cu.ExpectReadInstMem(4000, 8, nil, 0)

		scheduler.Schedule(0)

		cu.AllExpectedAccessed()
	})

	ginkgo.It("should mark fetched", func() {
		data := make([]byte, 8)
		wf := new(emu.WfScheduleInfo)

		scheduler.Fetched(wf, data)

		gomega.Expect(wf.State).To(gomega.Equal(emu.Fetched))
		gomega.Expect(wf.InstBuf).To(gomega.Equal(data))
	})

	ginkgo.It("should decode", func() {
		inst := disasm.NewInstruction()
		decoder.InstToReturn = inst

		wf := emu.NewWavefront()
		wf.FirstWiFlatID = 0
		scheduler.AddWf(wf)
		scheduler.Wfs[0].State = emu.Fetched

		scheduler.Schedule(0)

		gomega.Expect(scheduler.Wfs[0].State).To(gomega.Equal(emu.Running))
	})
})
