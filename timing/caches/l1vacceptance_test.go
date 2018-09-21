package caches

import (
	"math/rand"
	"os"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/acceptancetests"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/trace"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = PDescribe("L1v Stress Test", func() {
	var (
		engine          akita.Engine
		conn            *akita.DirectConnection
		agent           *acceptancetests.MemAccessAgent
		l1v             *L1VCache
		lowModuleFinder *cache.SingleLowModuleFinder
		dram            *mem.IdealMemController
	)

	BeforeEach(func() {
		rand.Seed(0)

		engine = akita.NewSerialEngine()
		conn = akita.NewDirectConnection(engine)

		dram = mem.NewIdealMemController("dram", engine, 1*mem.GB)
		lowModuleFinder = new(cache.SingleLowModuleFinder)
		lowModuleFinder.LowModule = dram.ToTop

		l1v = BuildL1VCache("cache", engine, 1*akita.GHz, 1,
			6, 4, 14, lowModuleFinder)

		agent = acceptancetests.NewMemAccessAgent(engine)
		agent.WriteLeft = 1000
		agent.ReadLeft = 1000
		agent.LowModule = l1v.ToCU

		conn.PlugIn(agent.ToMem)
		conn.PlugIn(l1v.ToCU)
		conn.PlugIn(l1v.ToL2)
		conn.PlugIn(dram.ToTop)
	})

	It("should read and write with in the same cache line", func() {
		traceFile, _ := os.Create("l1_same_line.log")
		tracer := trace.NewTracer(traceFile)
		l1v.AcceptHook(tracer)

		agent.MaxAddress = 0x40

		agent.TickLater(0)
		engine.Run()

		Expect(agent.PendingReadReq).To(HaveLen(0))
		Expect(agent.PendingWriteReq).To(HaveLen(0))
	})

	It("should read and write with in the some cache lines", func() {
		traceFile, _ := os.Create("l1_1K.log")
		tracer := trace.NewTracer(traceFile)
		l1v.AcceptHook(tracer)

		agent.MaxAddress = 1024

		agent.TickLater(0)
		engine.Run()

		Expect(agent.PendingReadReq).To(HaveLen(0))
		Expect(agent.PendingWriteReq).To(HaveLen(0))
	})

	It("should read and write in a large address range", func() {
		traceFile, _ := os.Create("l1_1M.log")
		tracer := trace.NewTracer(traceFile)
		l1v.AcceptHook(tracer)

		agent.MaxAddress = 1048576

		agent.TickLater(0)
		engine.Run()

		Expect(agent.PendingReadReq).To(HaveLen(0))
		Expect(agent.PendingWriteReq).To(HaveLen(0))
	})
})
