package mesh

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/acceptance"
)

func Example() {
	meshWidth := 5
	meshHeight := 5
	numMessages := 2000

	// monitor := monitoring.NewMonitor()
	// monitor.StartServer()

	test := acceptance.NewTest()
	engine := sim.NewSerialEngine()
	// monitor.RegisterEngine(engine)
	freq := 1 * sim.GHz
	connector := NewConnector().
		// WithMonitor(monitor).
		WithEngine(engine).
		WithFreq(freq)

	connector.CreateNetwork("Mesh")

	for x := 0; x < meshWidth; x++ {
		for y := 0; y < meshHeight; y++ {
			name := fmt.Sprintf("Agent[%d][%d]", x, y)
			agent := acceptance.NewAgent(engine, freq, name, 4, test)
			agent.TickLater(0)

			// monitor.RegisterComponent(agent)

			connector.AddTile([3]int{x, y, 0}, agent.AgentPorts)
			test.RegisterAgent(agent)
		}
	}

	connector.EstablishNetwork()

	test.GenerateMsgs(uint64(numMessages))

	engine.Run()

	test.MustHaveReceivedAllMsgs()
	fmt.Println("passed!")

	// Output: passed!
}
