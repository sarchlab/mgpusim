package simplebankedmemory

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// assignPort builds a port instance for a declared component port and attaches
// it, using the same registrar the component builder used.
func assignPort(
	reg modeling.Registrar,
	comp *Comp,
	name string,
	bufSize int,
) {
	p := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, p)
}

type loopbackConnection struct {
	hooking.HookableBase

	name  string
	ports []messaging.Port
}

func newLoopbackConnection(name string) *loopbackConnection {
	return &loopbackConnection{
		name: name,
	}
}

func (c *loopbackConnection) Name() string {
	return c.name
}

func (c *loopbackConnection) PlugIn(port messaging.Port) {
	c.ports = append(c.ports, port)
	port.SetConnection(c)
}

func (c *loopbackConnection) Unplug(messaging.Port) {
	panic("not implemented")
}

func (c *loopbackConnection) NotifyAvailable(messaging.Port) {
	// No-op for the tests.
}

func (c *loopbackConnection) NotifySend() {
	c.transfer()
}

func (c *loopbackConnection) transfer() {
	if len(c.ports) != 2 {
		panic("loopbackConnection expects exactly two ports")
	}

	src := c.ports[0]
	dst := c.ports[1]
	c.forward(src, dst)
	c.forward(dst, src)
}

func (c *loopbackConnection) forward(src, dst messaging.Port) {
	for {
		msg := src.PeekOutgoing()
		if msg == nil {
			break
		}

		if !dst.CanDeliver() {
			break
		}

		dst.Deliver(msg)
		src.RetrieveOutgoing()
	}
}

type testAgent struct {
	hooking.HookableBase
	*messaging.PortOwnerBase

	name     string
	port     messaging.Port
	received []messaging.Msg
}

func newTestAgent(name string) *testAgent {
	naming.MustBeValid(name)

	a := &testAgent{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		name:          name,
	}

	a.port = messaging.NewPort(a, 16, 16, fmt.Sprintf("%s.Port", name))
	a.DeclarePort("Port")
	a.AssignPort("Port", a.port)

	return a
}

func (a *testAgent) Name() string {
	return a.name
}

func (a *testAgent) NotifyRecv(port messaging.Port) {
	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			break
		}

		a.received = append(a.received, msg)
	}
}

func (a *testAgent) NotifyPortFree(messaging.Port) {
	// No-op.
}

func (a *testAgent) Handle(timing.Event) error {
	return nil
}

func (a *testAgent) send(msg messaging.Msg) {
	Expect(a.port.CanSend()).To(BeTrue())
	a.port.Send(msg)
}

func makeRead(
	src, dst messaging.RemotePort,
	addr, byteSize uint64,
) memprotocol.ReadReq {
	read := memprotocol.ReadReq{}
	read.ID = timing.GetIDGenerator().Generate()
	read.Src = src
	read.Dst = dst
	read.Address = addr
	read.AccessByteSize = byteSize
	read.TrafficBytes = 12
	read.TrafficClass = "memprotocol.ReadReq"
	return read
}

func makeWrite(
	src, dst messaging.RemotePort,
	addr uint64,
	data []byte,
) memprotocol.WriteReq {
	write := memprotocol.WriteReq{}
	write.ID = timing.GetIDGenerator().Generate()
	write.Src = src
	write.Dst = dst
	write.Address = addr
	write.Data = data
	write.TrafficBytes = len(data) + 12
	write.TrafficClass = "memprotocol.WriteReq"
	return write
}

var _ = Describe("SimpleBankedMemory", func() {
	var (
		engine  timing.Engine
		memComp *Comp
		storage *mem.Storage
		agent   *testAgent
		conn    *loopbackConnection
	)

	buildWithSpec := func(spec Spec, name string) {
		reg := modeling.NewStandaloneRegistrar(engine)
		memComp = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(Resources{Storage: storage}).
			Build(name)

		assignPort(reg, memComp, "Top", 16)
		assignPort(reg, memComp, "Control", 16)

		topPort := memComp.GetPortByName("Top")
		agent = newTestAgent(name + "Agent")
		conn = newLoopbackConnection(name + "Conn")
		conn.PlugIn(topPort)
		conn.PlugIn(agent.port)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		storage = mem.NewStorage(4 * mem.GB)

		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 2

		buildWithSpec(spec, "Mem")
	})

	AfterEach(func() {
		agent.received = nil
	})

	It("should pass ValidateState", func() {
		err := modeling.ValidateState(State{})
		Expect(err).To(Succeed())
	})

	It("should return read data after configured latency", func() {
		data := []byte{1, 2, 3, 4}
		err := storage.Write(0x0, data)
		Expect(err).NotTo(HaveOccurred())

		topPort := memComp.GetPortByName("Top")
		read := makeRead(
			agent.port.AsRemote(), topPort.AsRemote(), 0x0, uint64(len(data)))

		agent.send(read)

		for i := 0; i < 8; i++ {
			memComp.Tick()
		}

		Expect(agent.received).To(HaveLen(1))
		rsp := agent.received[0].(memprotocol.DataReadyRsp)
		Expect(rsp.Data).To(Equal(data))
	})

	It("should commit write before serving subsequent read", func() {
		addr := uint64(0x100)

		initial := []byte{0xAA, 0xBB, 0xCC, 0xDD}
		err := storage.Write(addr, initial)
		Expect(err).NotTo(HaveOccurred())

		newData := []byte{0x10, 0x20, 0x30, 0x40}

		topPort := memComp.GetPortByName("Top")

		write := makeWrite(
			agent.port.AsRemote(), topPort.AsRemote(), addr, newData)
		read := makeRead(
			agent.port.AsRemote(), topPort.AsRemote(), addr,
			uint64(len(newData)))

		agent.send(write)
		agent.send(read)

		for i := 0; i < 12; i++ {
			memComp.Tick()
		}

		Expect(agent.received).To(HaveLen(2))

		_, isWriteDone := agent.received[0].(memprotocol.WriteDoneRsp)
		Expect(isWriteDone).To(BeTrue())

		readRsp, ok := agent.received[1].(memprotocol.DataReadyRsp)
		Expect(ok).To(BeTrue())
		Expect(readRsp.Data).To(Equal(newData))

		committed, err := storage.Read(addr, uint64(len(newData)))
		Expect(err).NotTo(HaveOccurred())
		Expect(committed).To(Equal(newData))
	})

	It("should use the bank address conversion only for bank selection",
		func() {
			// The bank conversion maps the external address into the
			// element-local space (interleaving size 256, 2 elements, index
			// 0), but storage reads/writes use the raw external address.
			spec := DefaultSpec()
			spec.NumBanks = 2
			spec.StageLatency = 2
			spec.BankAddrConvKind = "interleaving"
			spec.BankAddrInterleavingSize = 256
			spec.BankAddrTotalNumOfElements = 2
			spec.BankAddrCurrentElementIndex = 0

			buildWithSpec(spec, "MemBankConv")

			data := []byte{5, 6, 7, 8}
			// External address 0x200 belongs to element 0 (0x200 % 512 < 256)
			// and is stored at the raw address.
			err := storage.Write(0x200, data)
			Expect(err).NotTo(HaveOccurred())

			topPort := memComp.GetPortByName("Top")
			read := makeRead(
				agent.port.AsRemote(), topPort.AsRemote(), 0x200,
				uint64(len(data)))

			agent.send(read)

			for i := 0; i < 8; i++ {
				memComp.Tick()
			}

			Expect(agent.received).To(HaveLen(1))
			rsp := agent.received[0].(memprotocol.DataReadyRsp)
			Expect(rsp.Data).To(Equal(data))
		})

	It("should delay row misses and serve row hits quickly", func() {
		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 1
		spec.BankPipelineDepth = 1
		spec.RowBufferSizeLog2 = 11
		spec.RowMissDelay = 20

		buildWithSpec(spec, "MemRow")

		data := []byte{1, 2, 3, 4}
		err := storage.Write(0x0, data)
		Expect(err).NotTo(HaveOccurred())

		topPort := memComp.GetPortByName("Top")

		// First access misses the (initially invalid) row buffer.
		read1 := makeRead(
			agent.port.AsRemote(), topPort.AsRemote(), 0x0, uint64(len(data)))
		agent.send(read1)

		missCycles := 0
		for len(agent.received) == 0 {
			memComp.Tick()
			missCycles++
			Expect(missCycles).To(BeNumerically("<", 100))
		}
		Expect(missCycles).To(BeNumerically(">=", spec.RowMissDelay))

		// Second access to the same row hits the row buffer.
		read2 := makeRead(
			agent.port.AsRemote(), topPort.AsRemote(), 0x0, uint64(len(data)))
		agent.send(read2)

		hitCycles := 0
		for len(agent.received) == 1 {
			memComp.Tick()
			hitCycles++
			Expect(hitCycles).To(BeNumerically("<", 100))
		}
		Expect(hitCycles).To(BeNumerically("<", spec.RowMissDelay))
	})

	It("should not head-of-line block requests to other banks", func() {
		// Bank 0's pipeline (width 1, depth 1) and post-pipeline buffer
		// (size 1) fill up quickly; a request to bank 1 queued behind many
		// bank-0 requests must still be served promptly.
		spec := DefaultSpec()
		spec.NumBanks = 2
		spec.StageLatency = 10
		spec.PostPipelineBufSize = 1

		buildWithSpec(spec, "MemHol")

		topPort := memComp.GetPortByName("Top")
		interleave := uint64(1) << spec.BankSelectorLog2InterleaveSize

		// Several requests to bank 0 (addresses with even interleave index).
		var bank0IDs []uint64
		for i := 0; i < 6; i++ {
			read := makeRead(
				agent.port.AsRemote(), topPort.AsRemote(),
				uint64(i)*2*interleave, 4)
			bank0IDs = append(bank0IDs, read.ID)
			agent.send(read)
		}

		// One request to bank 1 (odd interleave index), sent last.
		bank1Read := makeRead(
			agent.port.AsRemote(), topPort.AsRemote(), interleave, 4)
		agent.send(bank1Read)

		for i := 0; i < 15; i++ {
			memComp.Tick()
		}

		var receivedRspTos []uint64
		for _, msg := range agent.received {
			receivedRspTos = append(receivedRspTos, msg.Meta().RspTo)
		}

		// The bank-1 request completes even though several bank-0 requests
		// that arrived earlier are still pending.
		Expect(receivedRspTos).To(ContainElement(bank1Read.ID))
		Expect(receivedRspTos).NotTo(ContainElements(bank0IDs[4], bank0IDs[5]))
	})
})
