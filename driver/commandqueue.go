package driver

import (
	"encoding/binary"
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
)

// A Command is a task to execute later
type Command interface {
	GetReq() akita.Req
}

// A MemCopyH2DCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyH2DCommand struct {
	Dst GPUPtr
	Src interface{}
	Req *gcn3.MemCopyH2DReq
}

// GetReq returns the request assocated with the command
func (c *MemCopyH2DCommand) GetReq() akita.Req {
	return c.Req
}

// A MemCopyD2HCommand is a command that copies memory from the host to a
// GPU when the command is processed
type MemCopyD2HCommand struct {
	Dst interface{}
	Src GPUPtr
	Req *gcn3.MemCopyD2HReq
}

// GetReq returns the request assocated with the command
func (c *MemCopyD2HCommand) GetReq() akita.Req {
	return c.Req
}

// A KernelLaunchingCommand is a command will execute a kernel when it is
// processed.
type KernelLaunchingCommand struct {
	CodeObject *insts.HsaCo
	GridSize   [3]uint32
	WGSize     [3]uint16
	kernelArgs interface{}
}

// A CommandQueue maintains a queue of command where the commands from the
// queue will executes in order.
type CommandQueue struct {
	IsRunning bool
	GPUID     int
	Commands  []Command
}

// CreateCommandQueue creates a command queue in the driver
func (d *Driver) CreateCommandQueue() *CommandQueue {
	q := new(CommandQueue)
	q.GPUID = d.usingGPU
	d.CommandQueues = append(d.CommandQueues, q)
	return q
}

// A commandQueueDrainer runs all the commands in the command queue
type commandQueueDrainer interface {
	drain()
	scan()
}

type defaultCommandQueueDrainer struct {
	driver *Driver
	engine akita.Engine
}

func (d *defaultCommandQueueDrainer) drain() {
	d.scan()
	err := d.engine.Run()
	if err != nil {
		panic(err)
	}
}

func (d *defaultCommandQueueDrainer) scan() {
	for _, q := range d.driver.CommandQueues {
		if q.IsRunning {
			continue
		}

		if len(q.Commands) == 0 {
			continue
		}

		cmd := q.Commands[0]
		switch cmd := cmd.(type) {
		case *MemCopyD2HCommand:
			d.execMemD2HCommand(q, cmd)
		default:
			log.Panicf("cannot handle command of type %s", reflect.TypeOf(cmd))
		}
	}
}

func (d *defaultCommandQueueDrainer) execMemD2HCommand(
	queue *CommandQueue,
	cmd *MemCopyD2HCommand,
) {
	rawData := make([]byte, binary.Size(cmd.Dst))

	gpu := d.driver.gpus[queue.GPUID].ToDriver
	start := d.engine.CurrentTime() + 1e-8
	req := gcn3.NewMemCopyD2HReq(
		start,
		d.driver.ToGPUs,
		gpu,
		uint64(cmd.Src),
		rawData,
	)
	d.driver.ToGPUs.Send(req)
}
