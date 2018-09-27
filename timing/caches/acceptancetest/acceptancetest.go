package main

import (
	"sync"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/caches"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/acceptancetests"
	"gitlab.com/akita/mem/cache"
)

type test struct {
	engine          akita.Engine
	conn            *akita.DirectConnection
	agent           *acceptancetests.MemAccessAgent
	l1v             *caches.L1VCache
	lowModuleFinder *cache.SingleLowModuleFinder
	dram            *mem.IdealMemController
}

func (t *test) run(wg *sync.WaitGroup) {
	defer wg.Done()

	t.agent.TickLater(0)
	t.engine.Run()
}

func (t *test) setMaxAddr(addr uint64) {
	t.agent.MaxAddress = addr
}

func newTest() *test {
	t := new(test)

	t.engine = akita.NewSerialEngine()
	t.conn = akita.NewDirectConnection(t.engine)

	t.dram = mem.NewIdealMemController("dram", t.engine, 1*mem.GB)
	t.lowModuleFinder = new(cache.SingleLowModuleFinder)
	t.lowModuleFinder.LowModule = t.dram.ToTop

	t.l1v = caches.BuildL1VCache("cache", t.engine, 1*akita.GHz, 1,
		6, 4, 14, t.lowModuleFinder)

	t.agent = acceptancetests.NewMemAccessAgent(t.engine)
	t.agent.WriteLeft = 1000
	t.agent.ReadLeft = 1000
	t.agent.LowModule = t.l1v.ToCU

	t.conn.PlugIn(t.agent.ToMem)
	t.conn.PlugIn(t.l1v.ToCU)
	t.conn.PlugIn(t.l1v.ToL2)
	t.conn.PlugIn(t.dram.ToTop)

	return t
}

func main() {
	t1 := newTest()
	t1.setMaxAddr(64)

	t2 := newTest()
	t2.setMaxAddr(1024)

	t3 := newTest()
	t3.setMaxAddr(1048576)

	var wg sync.WaitGroup
	wg.Add(3)
	go t1.run(&wg)
	go t2.run(&wg)
	go t3.run(&wg)
	wg.Wait()
}
