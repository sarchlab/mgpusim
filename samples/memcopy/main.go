package main

import (
	"log"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/platform"
)

var (
	gpuDriver *driver.Driver
	size      uint64
)

func main() {
	_, _, gpuDriver = platform.BuildR9NanoPlatform()

	size = 1048576

	data := make([]byte, size)
	retData := make([]byte, size)
	for i := uint64(0); i < size; i++ {
		data[i] = byte(rand.Int())
	}

	gpuData := gpuDriver.AllocateMemory(size)

	gpuDriver.MemCopyH2D(gpuData, data)
	gpuDriver.MemCopyD2H(retData, gpuData)

	for i := uint64(0); i < size; i++ {
		if data[i] != retData[i] {
			log.Panicf("error at %d, expected %02x, but get %02x",
				i, data[i], retData[i])
		}
	}
	log.Printf("Passed!")

}
