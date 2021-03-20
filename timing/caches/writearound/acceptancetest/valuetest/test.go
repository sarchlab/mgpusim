package main

import (
	"sync"

	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/idealmemcontroller"

	"gitlab.com/akita/mem/v2/acceptancetests"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/mgpusim/v2/timing/caches/writearound"
)

type test struct {
	engine          sim.Engine
	conn            *sim.DirectConnection
	agent           *acceptancetests.MemAccessAgent
	lowModuleFinder *mem.SingleLowModuleFinder
	dram            *idealmemcontroller.Comp
	c               *writearound.Cache
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

	t.engine = sim.NewSerialEngine()
	t.conn = sim.NewDirectConnection("conn", t.engine, 1*sim.GHz)

	t.dram = idealmemcontroller.New("dram", t.engine, 1*mem.GB)
	t.lowModuleFinder = new(mem.SingleLowModuleFinder)
	t.lowModuleFinder.LowModule = t.dram.GetPortByName("Top")

	t.c = writearound.NewBuilder().
		WithEngine(t.engine).
		WithLowModuleFinder(t.lowModuleFinder).
		Build("cache")

	t.agent = acceptancetests.NewMemAccessAgent(t.engine)
	t.agent.WriteLeft = 1000
	t.agent.ReadLeft = 1000
	t.agent.LowModule = t.c.GetPortByName("Top")

	t.conn.PlugIn(t.agent.GetPortByName("Mem"), 4)
	t.conn.PlugIn(t.c.GetPortByName("Top"), 4)
	t.conn.PlugIn(t.c.GetPortByName("Bottom"), 16)
	t.conn.PlugIn(t.c.GetPortByName("Control"), 1)
	t.conn.PlugIn(t.dram.GetPortByName("Top"), 16)

	return t
}

func main() {
	var wg sync.WaitGroup

	//rand.Seed(1)

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
