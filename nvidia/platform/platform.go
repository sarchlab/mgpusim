package platform

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/driver"
	"github.com/sarchlab/mgpusim/v5/nvidia/gpu"
)

// Platform is a driver plus the GPUs it controls.
type Platform struct {
	Driver  *driver.Driver
	Devices []*gpu.GPUController
}

// Device describes a GPU model that the simulator can build.
type Device struct {
	Name string
	// Freq is the core clock. All components run at this clock.
	Freq timing.Freq
	// LaunchOverheadLatency is the number of cycles between two kernel
	// launches.
	LaunchOverheadLatency uint64
	GPU                   gpu.Config
}

// H100 returns the configuration of an NVIDIA H100 (PCIe) GPU.
func H100() Device {
	return Device{
		Name:                  "H100",
		Freq:                  1755 * timing.MHz,
		LaunchOverheadLatency: 2000,
		GPU: gpu.Config{
			NumSMs:                             114,
			NumSMSPsPerSM:                      4,
			L2CacheSize:                        50 * mem.MB,
			NumMemoryBanks:                     4,
			Log2CacheLineSize:                  9,
			L2BankLatency:                      305,
			DRAMLatency:                        490,
			SMThreadCapacity:                   2048,
			MaxCTAPerSM:                        16,
			GPU2SMThreadBlockAllocationLatency: 3,
			SMReceiveGPULatency:                5,
			GPUReceiveCTALatencyUnit:           0.10,
			CWDIssueWidth:                      8,
			SMResponseHandleWidth:              8,
			SMSPResponseHandleWidth:            8,
			MemResponseHandleWidth:             8,
		},
	}
}

// A100 returns the configuration of an NVIDIA A100 GPU.
func A100() Device {
	d := H100()
	d.Name = "A100"
	d.Freq = 1410 * timing.MHz
	d.GPU.NumSMs = 108
	d.GPU.L2CacheSize = 40 * mem.MB

	return d
}

// DeviceByName returns the configuration of the named GPU model.
func DeviceByName(name string) (Device, error) {
	switch strings.ToUpper(name) {
	case "H100":
		return H100(), nil
	case "A100":
		return A100(), nil
	default:
		return Device{}, fmt.Errorf("unknown device %q, expect H100 or A100",
			name)
	}
}

// Build creates a platform with one GPU of the given model.
func Build(registrar modeling.Registrar, device Device) *Platform {
	p := &Platform{}

	p.Driver = driver.MakeBuilder().
		WithRegistrar(registrar).
		WithFreq(device.Freq).
		WithLaunchOverheadLatency(device.LaunchOverheadLatency).
		Build("Driver")

	g := gpu.MakeBuilder().
		WithRegistrar(registrar).
		WithFreq(device.Freq).
		WithConfig(device.GPU).
		Build("GPU[0]")
	p.Driver.RegisterGPU(g)
	p.Devices = append(p.Devices, g)

	return p
}
