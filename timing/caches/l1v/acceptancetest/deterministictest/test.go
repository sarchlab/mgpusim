package main

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/idealmemcontroller"

	"gitlab.com/akita/mem/v2/acceptancetests"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/mgpusim/v2/timing/caches/l1v"
)

type test struct {
	engine          sim.Engine
	conn            *sim.DirectConnection
	agent           *acceptancetests.MemAccessAgent
	lowModuleFinder *mem.SingleLowModuleFinder
	dram            *idealmemcontroller.Comp
	c               *l1v.Cache
}

func (t *test) run(wg *sync.WaitGroup) {
	t.agent.TickLater(0)
	t.engine.Run()

	if wg != nil {
		wg.Done()
	}
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

	t.c = l1v.NewBuilder().
		WithEngine(t.engine).
		WithLowModuleFinder(t.lowModuleFinder).
		Build("cache")

	t.agent = acceptancetests.NewMemAccessAgent(t.engine)
	t.agent.WriteLeft = 10000
	t.agent.ReadLeft = 10000
	t.agent.LowModule = t.c.GetPortByName("Top")

	t.conn.PlugIn(t.agent.GetPortByName("Mem"), 4)
	t.conn.PlugIn(t.c.GetPortByName("Top"), 4)
	t.conn.PlugIn(t.c.GetPortByName("Bottom"), 16)
	t.conn.PlugIn(t.c.GetPortByName("Control"), 1)
	t.conn.PlugIn(t.dram.GetPortByName("Top"), 16)

	return t
}

func main() {
	seed := time.Now().UnixNano()
	log.Printf("seed %d\n", seed)

	rand.Seed(seed)
	t1 := newTest("Max_64")
	t1.setMaxAddr(64)
	t1.run(nil)

	rand.Seed(seed)
	t2 := newTest("Max_64")
	t2.setMaxAddr(64)
	t2.run(nil)

	log.Printf("t1 time %.10f, t2 time %.10f\n",
		t1.engine.CurrentTime(),
		t2.engine.CurrentTime())
	if t1.engine.CurrentTime() != t2.engine.CurrentTime() {
		panic("L1 cache is not deterministic")
	}

	rand.Seed(seed)
	t3 := newTest("Max_1024")
	t3.setMaxAddr(1024)
	t3.run(nil)

	rand.Seed(seed)
	t4 := newTest("Max_1024")
	t4.setMaxAddr(1024)
	t4.run(nil)

	log.Printf("t3 time %.10f, t3 time %.10f\n",
		t3.engine.CurrentTime(),
		t4.engine.CurrentTime())
	if t3.engine.CurrentTime() != t4.engine.CurrentTime() {
		panic("L1 cache is not deterministic")
	}

	rand.Seed(seed)
	t5 := newTest("Max_1048576")
	t5.setMaxAddr(1048576)
	t5.run(nil)

	rand.Seed(seed)
	t6 := newTest("Max_1048576")
	t6.setMaxAddr(1048576)
	t6.run(nil)

	log.Printf("t5 time %.10f, t6 time %.10f\n",
		t5.engine.CurrentTime(),
		t6.engine.CurrentTime())
	if t5.engine.CurrentTime() != t6.engine.CurrentTime() {
		panic("L1 cache is not deterministic")
	}
}
