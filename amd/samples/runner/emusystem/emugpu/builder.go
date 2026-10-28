// Package emugpu contains the configuration for the emulation of a GPU.
package emugpu

import (
	"fmt"
	"log"
	"os"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/emu/cdna3"
	"github.com/sarchlab/mgpusim/v5/amd/emu/gcn3"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// portBufSize is the buffer size used for the GPU-internal ports. The v4
// platform used very deep buffers; 4096 is plenty for the per-cycle traffic of
// the emulation platform while keeping memory usage reasonable.
const portBufSize = 4096

// GPU is the externally visible result of building an emulation GPU. The
// command processor port is the GPU's driver-facing port; the platform
// registers it with the driver and plugs it into the inter-device connection.
type GPU struct {
	Name                 string
	CommandProcessorPort messaging.Port
}

// Builder builds a GPU for emulation.
type Builder struct {
	simulation     *simulation.Simulation
	freq           timing.Freq
	log2PageSize   uint64
	enableISADebug bool
	pageTable      *vm.PageTable
	storage        *mem.Storage
	archType       arch.Type
}

// MakeBuilder creates a new Builder with default parameters.
func MakeBuilder() Builder {
	b := Builder{}

	b.freq = 1 * timing.GHz
	b.log2PageSize = 12
	b.enableISADebug = false

	return b
}

// WithSimulation sets the simulation to use. The simulation is used as the
// registrar for the built components, connections, and ports.
func (b Builder) WithSimulation(s *simulation.Simulation) Builder {
	b.simulation = s
	return b
}

// WithPageTable sets the page table that provides the address translation.
func (b Builder) WithPageTable(pageTable *vm.PageTable) Builder {
	b.pageTable = pageTable
	return b
}

// WithLog2PageSize sets the page size of the GPU, as a power of 2.
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	return b
}

// WithStorage sets the global memory storage that is shared by multiple GPUs
// and the driver.
func (b Builder) WithStorage(s *mem.Storage) Builder {
	b.storage = s
	return b
}

// WithISADebugging enables the simulation to dump instruction execution
// information.
func (b Builder) WithISADebugging() Builder {
	b.enableISADebug = true
	return b
}

// WithArchitecture sets the GPU architecture for emulation.
func (b Builder) WithArchitecture(archType arch.Type) Builder {
	b.archType = archType
	return b
}

// Build builds the GPU and returns its externally visible handle.
func (b Builder) Build(name string) *GPU {
	gpuMem := b.buildMemory(name)
	cu := b.buildComputeUnit(name)
	cp, cpDriverPort := b.buildCommandProcessor(name, cu)

	b.connectInternalComponents(name, cp, cu, gpuMem)

	return &GPU{
		Name:                 name,
		CommandProcessorPort: cpDriverPort,
	}
}

func (b Builder) buildMemory(name string) *idealmemcontroller.Comp {
	spec := idealmemcontroller.DefaultSpec()
	spec.Freq = b.freq
	spec.Latency = 1

	gpuMem := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).
		WithSimulation(b.simulation).
		WithResources(idealmemcontroller.Resources{Storage: b.storage}).
		Build(name + ".GlobalMem")

	b.assignPort(gpuMem, "Top")
	b.assignPort(gpuMem, "Control")

	return gpuMem
}

func (b Builder) buildComputeUnit(
	name string,
) *emu.Comp {
	isCDNA3 := b.archType == arch.CDNA3

	disassembler := insts.NewDisassembler()
	disassembler.IsCDNA3 = isCDNA3

	storageAccessor := emu.NewStorageAccessor(
		b.storage,
		b.pageTable,
		b.log2PageSize,
		nil,
	)

	var alu emu.ALU
	if isCDNA3 {
		alu = cdna3.NewALU(storageAccessor)
	} else {
		alu = gcn3.NewALU(storageAccessor)
	}

	spec := emu.DefaultSpec()
	spec.Freq = b.freq

	cu := emu.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(spec).
		WithResources(emu.Resources{
			Decoder:         disassembler,
			ALU:             alu,
			StorageAccessor: storageAccessor,
		}).
		Build(name + ".CU")

	b.assignPort(cu, emu.DispatchPortName)

	if b.enableISADebug {
		isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", cu.Name()))
		if err != nil {
			log.Fatal(err.Error())
		}

		isaDebugger := emu.NewISADebugger(log.New(isaDebug, "", 0))
		cu.AcceptHook(isaDebugger)
	}

	return cu
}

func (b Builder) buildCommandProcessor(
	name string,
	cu *emu.Comp,
) (*Comp, messaging.Port) {
	cp := buildCommandProcessor(
		b.simulation, b.freq, cu.Ports.ToDispatcher.AsRemote(), name+".CommandProcessor")

	driverPort := b.assignPort(cp, cpDriverPortName)
	b.assignPort(cp, cpCUPortName)

	return cp, driverPort
}

func (b Builder) connectInternalComponents(
	name string,
	cp *Comp,
	cu *emu.Comp,
	gpuMem *idealmemcontroller.Comp,
) {
	conn := directconnection.NewConnection(name+".IntraGPUConn", b.simulation, b.freq)

	conn.BindPort(cp.Ports.ToCU)
	conn.BindPort(cu.Ports.ToDispatcher)
	conn.BindPort(gpuMem.Ports.Top)
}

// assignPort builds a port for the given declared port name, assigns it to the
// component, and returns it.
func (b Builder) assignPort(comp interface{ BindPort(string, messaging.Port) }, portName string) messaging.Port {
	port := twowaybuffered.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: portBufSize, OutgoingCapacity: portBufSize}).
		Build()
	comp.BindPort(portName, port)
	return port
}
