package timingconfig

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/simplebankedmemory"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

const cpuMemoryTop = messaging.RemotePort("CPU.Memory.Top")

// hostMemoryLatencyNs is the idle access latency of the host memory. It is an
// uncalibrated placeholder.
const hostMemoryLatencyNs = 100

// HostMemorySpec configures the host DRAM. Each channel is one bank that
// starts one 64 B request per cycle of Freq, so the peak bandwidth is
// Channels * 64 B * Freq.
type HostMemorySpec struct {
	Channels  int
	Freq      timing.Freq
	LatencyNs int
}

// R9NanoHostMemorySpec is four channels of DDR4-2400, 76.8 GB/s at peak.
func R9NanoHostMemorySpec() HostMemorySpec {
	return HostMemorySpec{Channels: 4, Freq: 300 * timing.MHz, LatencyNs: hostMemoryLatencyNs}
}

// MI300XHostMemorySpec is twelve channels of DDR5-4800, 460.8 GB/s at peak.
func MI300XHostMemorySpec() HostMemorySpec {
	return HostMemorySpec{Channels: 12, Freq: 600 * timing.MHz, LatencyNs: hostMemoryLatencyNs}
}

// WithHostMemory sets the host memory. The default depends on the GPU type.
func (b Builder) WithHostMemory(spec HostMemorySpec) Builder {
	b.hostMemory = &spec
	return b
}

// hostMemorySpec returns the configured host memory or the platform default.
// A wafer-scale platform uses the MI300X host memory.
func (b *Builder) hostMemorySpec() HostMemorySpec {
	if b.hostMemory != nil {
		return *b.hostMemory
	}
	if _, ok := waferscale.ParseGPUType(b.gpuType); ok || b.gpuType == "mi300x" {
		return MI300XHostMemorySpec()
	}
	return R9NanoHostMemorySpec()
}

// bankedSpec converts the host memory to the Akita component configuration.
func (s HostMemorySpec) bankedSpec() simplebankedmemory.Spec {
	if s.Channels <= 0 || s.Freq == 0 || s.LatencyNs <= 0 {
		panic(fmt.Sprintf("timingconfig: invalid host memory %+v", s))
	}
	spec := simplebankedmemory.DefaultSpec()
	spec.Freq = s.Freq
	spec.NumBanks = s.Channels
	spec.BankPipelineWidth = 1
	spec.StageLatency = 1
	spec.BankPipelineDepth = int((uint64(s.LatencyNs)*uint64(s.Freq) + uint64(timing.GHz) - 1) / uint64(timing.GHz))
	spec.BankSelectorLog2InterleaveSize = 6
	return spec
}

func (b *Builder) buildCPUMemory(network interDeviceNetwork) {
	memory := modeling.MakeBuilder[*simplebankedmemory.Comp](b.hostMemorySpec().bankedSpec()).
		WithSimulation(b.simulation).
		WithResources(simplebankedmemory.Resources{Storage: b.globalStorage}).
		Build("CPU.Memory")
	network.BindHostMemoryPort(b.buildPort(memory, "Top", mmuTopPortBufSize))
	b.buildPort(memory, "Control", ctrlPortBufSize)
}
