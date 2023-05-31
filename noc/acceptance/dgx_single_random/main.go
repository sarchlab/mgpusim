package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"

	// Enable profiling
	_ "net/http/pprof"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/acceptance"
	"github.com/sarchlab/mgpusim/v3/noc/networking/nvlink"
	"github.com/tebeka/atexit"
)

func startProfilingServer() {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	fmt.Println("Profiling server running on:",
		listener.Addr().(*net.TCPAddr).Port)

	panic(http.Serve(listener, nil))
}

func main() {
	flag.Parse()

	go startProfilingServer()

	for i := 0; i < 9; i++ {
		for j := 0; j < 9; j++ {
			fmt.Printf("Testing P2P between agent %v and agent %v\n", i, j)
			rand.Seed(1)

			engine := sim.NewSerialEngine()
			t := acceptance.NewTest()

			agents := createNetwork(engine, t)
			t.RegisterAgent(agents[i])
			t.RegisterAgent(agents[j])
			t.GenerateMsgs(2000)

			engine.Run()

			t.MustHaveReceivedAllMsgs()
			t.ReportBandwidthAchieved(engine.CurrentTime())
		}
	}

	atexit.Exit(0)
}

func createNetwork(
	engine sim.Engine,
	test *acceptance.Test,
) []*acceptance.Agent {
	// visTracer := tracing.NewMySQLTracer()
	// visTracer.Init()
	agents := createAgents(engine, test)

	connector := nvlink.NewConnector().
		WithEngine(engine).
		WithPCIeVersion(3, 16)
	connector.CreateNetwork("Network")

	deviceIDs := createPCIeNetwork(connector, agents)
	createNVLinkNetwork(connector, deviceIDs)

	connector.EstablishRoute()

	return agents
}

func createAgents(engine sim.Engine, test *acceptance.Test) []*acceptance.Agent {
	freq := 1.0 * sim.GHz
	var agents []*acceptance.Agent
	for i := 0; i < 9; i++ {
		agent := acceptance.NewAgent(
			engine, freq, fmt.Sprintf("Agent%d", i), 1, test)
		agent.TickLater(0)
		agents = append(agents, agent)
		//test.RegisterAgent(agent)
	}
	return agents
}

func createPCIeNetwork(connector *nvlink.Connector, agents []*acceptance.Agent) []int {
	rootComplexID := connector.AddRootComplex(agents[0].AgentPorts)
	switch1ID := connector.AddPCIeSwitch()
	switch2ID := connector.AddPCIeSwitch()

	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch1ID)
	connector.ConnectSwitchesWithPCIeLink(rootComplexID, switch2ID)

	deviceIDs := []int{0}

	for i := 1; i < 5; i++ {
		deviceID := connector.PlugInDevice(switch1ID, agents[i].AgentPorts)
		deviceIDs = append(deviceIDs, deviceID)
	}

	for i := 5; i < 9; i++ {
		deviceID := connector.PlugInDevice(switch2ID, agents[i].AgentPorts)
		deviceIDs = append(deviceIDs, deviceID)
	}
	return deviceIDs
}

func createNVLinkNetwork(connector *nvlink.Connector, deviceIDs []int) {
	connector.ConnectDevicesWithNVLink(deviceIDs[1], deviceIDs[2], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[2], deviceIDs[3], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[3], deviceIDs[4], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[1], deviceIDs[4], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[1], deviceIDs[3], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[2], deviceIDs[4], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[5], deviceIDs[6], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[6], deviceIDs[7], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[7], deviceIDs[8], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[6], deviceIDs[8], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[5], deviceIDs[7], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[6], deviceIDs[8], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[1], deviceIDs[6], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[2], deviceIDs[5], 2)
	connector.ConnectDevicesWithNVLink(deviceIDs[4], deviceIDs[8], 1)
	connector.ConnectDevicesWithNVLink(deviceIDs[3], deviceIDs[7], 2)
}
