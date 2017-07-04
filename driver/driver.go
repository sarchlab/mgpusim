package driver

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"encoding/binary"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
)

// A Driver of the GCN3Sim is a Yaotsu component that receives requests from
// the runtime and directly controls the simulator.
type Driver struct {
	*core.ComponentBase

	GPUs []*gcn3.GPU
}

// NewDriver returns a newly created driver.
func NewDriver(name string) *Driver {
	d := new(Driver)
	d.ComponentBase = core.NewComponentBase(name)

	d.GPUs = make([]*gcn3.GPU, 0)

	return d
}

// Listen wait for the clients to connect in.
func (d *Driver) Listen() {
	l, err := net.Listen("tcp", "localhost:13000")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	for {
		// Listen for an incoming connection.
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		// Handle connections in a new goroutine.
		go d.handleConnection(conn)
	}
}

func (d *Driver) handleConnection(conn net.Conn) {
	for {
		numberBuf := make([]byte, 8)
		lengthBuf := make([]byte, 8)
		var args []byte

		if _, err := io.ReadFull(conn, numberBuf); err != nil {
			log.Print(err)
			break
		}
		number := insts.BytesToUint64(numberBuf)

		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			log.Print(err)
			break
		}
		length := insts.BytesToUint64(lengthBuf)

		if length > 0 {
			args = make([]byte, length)
			if _, err := io.ReadFull(conn, args); err != nil {
				log.Print(err)
				break
			}
		}

		d.handleIOCTL(number, args, conn)
	}
}

func (d *Driver) handleIOCTL(number uint64, args []byte, conn net.Conn) {
	switch number {
	case 0x01:
		d.handleIOCTLGetVersion(conn)
	case 0x05:
		d.handleIOCTLGetClockCounters(args, conn)
	case 0x21:
		d.handleIOCTLAcquireSystemProperties(conn)
	case 0x22:
		d.handleIOCTLGetNodeProperties(args, conn)
	case 0x23:
		d.handleIOCTLGetNodeMemProperties(args, conn)
	default:
		log.Printf("IOCTL number 0x%02x is not supported.", number)
	}
}

type kfdIOCTLGetVersionArgs struct {
	majorVersion, minorVersion uint32
}

// IOCTL 0x01
func (d *Driver) handleIOCTLGetVersion(conn net.Conn) {
	args := new(kfdIOCTLGetVersionArgs)
	args.majorVersion = 1
	args.minorVersion = 0

	binary.Write(conn, binary.LittleEndian, args)
}

type kfdIOCTLGetClockCounters struct {
	GPUClockCounter, CPUClockCounter, SystemClockCounter, SystemClockFreq uint64
	NodeID, Pad                                                           uint32
}

// IOCTL 0x05
func (d *Driver) handleIOCTLGetClockCounters(args []byte, conn net.Conn) {
	prop := new(kfdIOCTLGetClockCounters)
	binary.Read(bytes.NewReader(args), binary.LittleEndian, prop)

	node := d.GPUs[prop.NodeID]
	prop.SystemClockFreq = uint64(node.Freq)

	binary.Write(conn, binary.LittleEndian, prop)
}

type kfdIOCTLAcquireSystemProperties struct {
	numNodes uint32
}

// IOCTL 0x21
func (d *Driver) handleIOCTLAcquireSystemProperties(conn net.Conn) {
	args := new(kfdIOCTLAcquireSystemProperties)
	args.numNodes = uint32(len(d.GPUs))

	binary.Write(conn, binary.LittleEndian, args)
}

type kfdIOCTLGetNodeProperties struct {
	NodeID           uint32
	NumFComputeCores uint32
	NumSIMDPerCU     uint32
	EngineID         uint32
	NumMemBanks      uint32
}

// IOCTL 0x22
func (d *Driver) handleIOCTLGetNodeProperties(
	args []byte,
	conn net.Conn,
) {
	prop := new(kfdIOCTLGetNodeProperties)
	binary.Read(bytes.NewReader(args), binary.LittleEndian, prop)

	node := d.GPUs[prop.NodeID]
	prop.NumFComputeCores = uint32(len(node.CUs)) * 4
	prop.NumSIMDPerCU = 4
	prop.EngineID = 8<<10 + 0<<16 + 3<<24 // GFX 803
	prop.NumMemBanks = 4                  // GPU DRAM, LDS, SCRATCH, SVM aperture

	binary.Write(conn, binary.LittleEndian, prop)
}

type kfdIOCTLGetNodeMemoryProperties struct {
	NodeID             uint32
	BankID             uint32
	HeapType           uint32
	Flags              uint32
	Width              uint32
	MaxClockMHz        uint32
	ByteSize           uint64
	VirtualBaseAddress uint64
}

func (d *Driver) handleIOCTLGetNodeMemProperties(
	args []byte,
	conn net.Conn,
) {
	prop := new(kfdIOCTLGetNodeMemoryProperties)
	binary.Read(bytes.NewReader(args), binary.LittleEndian, prop)
	log.Printf("bankid: %d\n", prop.BankID)

	switch prop.BankID {
	case 0: // GPU Main Memory
		prop.HeapType = 2 // Private
		prop.Flags = 0
		prop.Width = 512
		prop.MaxClockMHz = 500
		prop.ByteSize = 4 << 30 // 4 GB
		prop.VirtualBaseAddress = 0
	case 1:
		prop.HeapType = 4 // LDS
	case 2:
		prop.HeapType = 5 // SCRATCH
	case 3:
		prop.HeapType = 6 // SVM
	default:
		log.Fatalf("Not sure about what memory bank %d is", prop.BankID)
	}

	binary.Write(conn, binary.LittleEndian, prop)
}
