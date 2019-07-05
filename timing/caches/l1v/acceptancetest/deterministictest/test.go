package main

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"gitlab.com/akita/mem/idealmemcontroller"
	"gitlab.com/akita/mem/vm"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/caches/l1v"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/acceptancetests"
	"gitlab.com/akita/mem/cache"
)

type test struct {
	engine           akita.Engine
	conn             *akita.DirectConnection
	agent            *acceptancetests.MemAccessAgent
	lowModuleFinder  *cache.SingleLowModuleFinder
	dram             *idealmemcontroller.Comp
	pageTableFactory vm.PageTableFactory
	c                *l1v.Cache
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

	t.engine = akita.NewSerialEngine()
	t.conn = akita.NewDirectConnection(t.engine)

	t.dram = idealmemcontroller.New("dram", t.engine, 1*mem.GB)
	t.lowModuleFinder = new(cache.SingleLowModuleFinder)
	t.lowModuleFinder.LowModule = t.dram.ToTop

	t.pageTableFactory = new(vm.DefaultPageTableFactory)

	t.c = l1v.NewBuilder().
		WithEngine(t.engine).
		WithLowModuleFinder(t.lowModuleFinder).
		Build("cache")

	t.agent = acceptancetests.NewMemAccessAgent(t.engine)
	t.agent.WriteLeft = 10000
	t.agent.ReadLeft = 10000
	t.agent.LowModule = t.c.TopPort

	t.conn.PlugIn(t.agent.ToMem)
	t.conn.PlugIn(t.c.TopPort)
	t.conn.PlugIn(t.c.BottomPort)
	t.conn.PlugIn(t.c.ControlPort)
	t.conn.PlugIn(t.dram.ToTop)

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
