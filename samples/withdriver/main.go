package main

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/emu"
)

func main() {
	driver := driver.NewDriver("driver")

	gpu := createGPU()
	driver.GPUs = append(driver.GPUs, gpu)
	driver.Listen()
}

func createGPU() *gcn3.GPU {
	gpu := gcn3.NewGPU("GPU0")
	gpu.Freq = 1 * core.GHz
	for i := 0; i < 4; i++ {
		cu := emu.NewComputeUnit("GPU0.CU"+string(i), nil, nil, nil, nil)
		gpu.CUs = append(gpu.CUs, cu)
	}

	return gpu
}
