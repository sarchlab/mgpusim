package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/acceptance"
	"github.com/sarchlab/mgpusim/v3/noc/networking/pcie"
	"github.com/tebeka/atexit"
)

var numDevicePerSwitch = 8
var numPortPerDevice = 9

func main() {
	flag.Parse()
	rand.Seed(1)

	engine := sim.NewSerialEngine()
	// engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	t := acceptance.NewTest()

	createNetwork(engine, t)
	t.GenerateMsgs(10000)

	engine.Run()

	t.MustHaveReceivedAllMsgs()
	t.ReportBandwidthAchieved(engine.CurrentTime())
	atexit.Exit(0)
}

func createNetwork(engine sim.Engine, test *acceptance.Test) {
	monitor := monitoring.NewMonitor()
	monitor.RegisterEngine(engine)
	monitor.StartServer()

	freq := 1.0 * sim.GHz
	var agents []*acceptance.Agent
	for i := 0; i < numDevicePerSwitch*2+1; i++ {
		agent := acceptance.NewAgent(
			engine, freq, fmt.Sprintf("Agent%d", i), numPortPerDevice, test)
		agent.TickLater(0)
		agents = append(agents, agent)
		test.RegisterAgent(agent)
		monitor.RegisterComponent(agent)
	}

	pcieConnector := pcie.NewConnector()
	pcieConnector = pcieConnector.
		WithEngine(engine).
		WithFrequency(freq).
		WithMonitor(monitor).
		WithVersion(4, 16)

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(agents[0].AgentPorts)
	switch1ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch1ID, agents[i+1].AgentPorts)
	}

	switch2ID := pcieConnector.AddSwitch(rootComplexID)
	for i := 0; i < numDevicePerSwitch; i++ {
		pcieConnector.PlugInDevice(switch2ID,
			agents[i+1+numDevicePerSwitch].AgentPorts)
	}

	pcieConnector.EstablishRoute()
}
