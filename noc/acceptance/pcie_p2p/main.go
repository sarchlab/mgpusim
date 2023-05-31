package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/acceptance"
	"github.com/sarchlab/mgpusim/v3/noc/networking/pcie"
	"github.com/tebeka/atexit"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := sim.NewSerialEngine()
	t := acceptance.NewTest()

	createNetwork(engine, t)
	t.GenerateMsgs(1000)

	engine.Run()

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	atexit.Exit(0)
}

func createNetwork(engine sim.Engine, test *acceptance.Test) {
	freq := 1.0 * sim.GHz
	var agents []*acceptance.Agent
	for i := 0; i < 9; i++ {
		agent := acceptance.NewAgent(
			engine, freq, fmt.Sprintf("Agent%d", i), 5, test)
		agent.TickLater(0)
		agents = append(agents, agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithEngine(engine).
		WithFrequency(1*sim.GHz).
		WithVersion(4, 16)

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(agents[0].AgentPorts)

	switch1ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 1; i < 5; i++ {
		pcieConnector.PlugInDevice(switch1ID, agents[i].AgentPorts)
	}

	switch2ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 5; i < 9; i++ {
		pcieConnector.PlugInDevice(switch2ID, agents[i].AgentPorts)
	}

	pcieConnector.EstablishRoute()

	test.RegisterAgent(agents[1])
	test.RegisterAgent(agents[8])
}
