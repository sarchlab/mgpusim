package mesh_test

import (
	. "github.com/onsi/ginkgo/v2"
	// . "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/noc/networking/mesh"
)

var _ = Describe("Connector", func() {
	var (
		engine    sim.Engine
		connector *mesh.Connector
	)

	BeforeEach(func() {
		engine = sim.NewSerialEngine()
		connector = mesh.NewConnector().WithEngine(engine)
		connector.CreateNetwork("Network")
	})

	It("should be able to connect ports outside current capacity", func() {
		port := sim.NewLimitNumMsgPort(nil, 1, "Port")

		// 8,8,2 is the default capacity
		connector.AddTile([3]int{8, 8, 2}, []sim.Port{port})

		connector.EstablishNetwork()
	})
})
