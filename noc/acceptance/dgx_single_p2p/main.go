package main

import (
	"flag"
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/acceptance"
	"github.com/sarchlab/mgpusim/v3/noc/networking/nvlink"
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

	connector := nvlink.NewConnector().
		WithEngine(engine)

	connector.CreateNetwork("Network")

	rootComplexID := connector.AddRootComplex(agents[8].AgentPorts)
	switch1ID := connector.AddPCIeSwitch()
	switch2ID := connector.AddPCIeSwitch()

	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch1ID)
	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch2ID)

	for i := 0; i < 4; i++ {
		connector.PlugInDevice(switch1ID, agents[i].AgentPorts)
	}

	for i := 4; i < 8; i++ {
		connector.PlugInDevice(switch2ID, agents[i].AgentPorts)
	}

	connector.ConnectDevicesWithNVLink(0, 1, 1)
	connector.ConnectDevicesWithNVLink(0, 2, 1)
	connector.ConnectDevicesWithNVLink(1, 3, 1)
	connector.ConnectDevicesWithNVLink(2, 3, 1)
	connector.ConnectDevicesWithNVLink(0, 3, 1)
	connector.ConnectDevicesWithNVLink(1, 2, 1)
	connector.ConnectDevicesWithNVLink(5, 4, 1)
	connector.ConnectDevicesWithNVLink(5, 7, 1)
	connector.ConnectDevicesWithNVLink(7, 6, 1)
	connector.ConnectDevicesWithNVLink(4, 6, 1)
	connector.ConnectDevicesWithNVLink(4, 7, 1)
	connector.ConnectDevicesWithNVLink(5, 6, 1)
	connector.ConnectDevicesWithNVLink(0, 4, 1)
	connector.ConnectDevicesWithNVLink(1, 5, 1)
	connector.ConnectDevicesWithNVLink(2, 6, 1)
	connector.ConnectDevicesWithNVLink(3, 7, 1)

	connector.EstablishRoute()

	test.RegisterAgent(agents[0])
	test.RegisterAgent(agents[7])
}
