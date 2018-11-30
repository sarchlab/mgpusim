package main

import (
	"os"
	"sync"

	"gitlab.com/akita/mem/vm"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/caches"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/acceptancetests"
	"gitlab.com/akita/mem/cache"
	memTrace "gitlab.com/akita/mem/trace"
)

type test struct {
	engine          akita.Engine
	conn            *akita.DirectConnection
	agent           *acceptancetests.MemAccessAgent
	l1v             *caches.L1VCache
	lowModuleFinder *cache.SingleLowModuleFinder
	dram            *mem.IdealMemController
	mmu             *vm.MMUImpl
}

func (t *test) run(wg *sync.WaitGroup) {
	defer wg.Done()

	t.agent.TickLater(0)
	t.engine.Run()
}

func (t *test) setMaxAddr(addr uint64) {
	t.agent.MaxAddress = addr
}

func newTest(name string) *test {
	t := new(test)

	t.engine = akita.NewSerialEngine()
	t.conn = akita.NewDirectConnection(t.engine)

	t.dram = mem.NewIdealMemController("dram", t.engine, 1*mem.GB)
	t.lowModuleFinder = new(cache.SingleLowModuleFinder)
	t.lowModuleFinder.LowModule = t.dram.ToTop

	t.mmu = vm.NewMMU("mmu", t.engine)
	for addr := uint64(0); addr < mem.MB; addr += 4096 {
		t.mmu.CreatePage(&vm.Page{
			PID:      1,
			VAddr:    addr,
			PAddr:    addr,
			PageSize: 4096,
			Valid:    true,
		})
	}

	t.l1v = caches.BuildL1VCache("cache", t.engine, 1*akita.GHz, 1,
		6, 4, 14, t.lowModuleFinder, t.mmu.ToTop)

	traceFile, err := os.Create(name + ".trace")
	if err != nil {
		panic(err)
	}
	tracer := memTrace.NewTracer(traceFile)
	t.l1v.AcceptHook(tracer)

	t.agent = acceptancetests.NewMemAccessAgent(t.engine)
	t.agent.WriteLeft = 1000
	t.agent.ReadLeft = 1000
	t.agent.LowModule = t.l1v.ToCU

	t.conn.PlugIn(t.agent.ToMem)
	t.conn.PlugIn(t.l1v.ToCU)
	t.conn.PlugIn(t.l1v.ToL2)
	t.conn.PlugIn(t.l1v.ToTLB)
	t.conn.PlugIn(t.dram.ToTop)
	t.conn.PlugIn(t.mmu.ToTop)

	return t
}

func main() {
	var wg sync.WaitGroup

	t1 := newTest("Max_64")
	t1.setMaxAddr(64)
	wg.Add(1)

	t2 := newTest("Max_1024")
	t2.setMaxAddr(1024)
	wg.Add(1)

	t3 := newTest("Max_1M")
	t3.setMaxAddr(1048576)
	wg.Add(1)

	go t1.run(&wg)
	go t2.run(&wg)
	go t3.run(&wg)
	wg.Wait()
}
