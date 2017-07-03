package driver

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"encoding/binary"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

// A Driver of the GCN3Sim is a Yaotsu component that receives requests from
// the runtime and directly controls the simulator.
type Driver struct {
	*core.ComponentBase
}

// NewDriver returns a newly created driver.
func NewDriver(name string) *Driver {
	d := new(Driver)
	d.ComponentBase = core.NewComponentBase(name)
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
		var arg []byte

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
			arg = make([]byte, length)
			if _, err := io.ReadFull(conn, arg); err != nil {
				log.Print(err)
				break
			}
		}

		d.handleIOCTL(number, arg, conn)
	}
}

func (d *Driver) handleIOCTL(number uint64, arg []byte, conn net.Conn) {
	switch number {
	case 0x01:
		d.handleIOCTLGetVersion(conn)
	default:
		log.Printf("IOCTL number %d is not supported.", number)
	}
}

type kfdIOCTLGetVersionArgs struct {
	majorVersion, minorVersion uint32
}

// IOCTL 0x01
func (d *Driver) handleIOCTLGetVersion(conn net.Conn) {
	v := new(kfdIOCTLGetVersionArgs)
	v.majorVersion = 1
	v.minorVersion = 0

	binary.Write(conn, binary.LittleEndian, v)
}
