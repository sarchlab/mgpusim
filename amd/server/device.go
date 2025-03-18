package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type deviceCountRsp struct {
	DeviceCount int `json:"device_count"`
}

func handleDeviceCount(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handleDeviceCount")

	rsp := deviceCountRsp{}
	rsp.DeviceCount = len(serverInstance.driver.GPUs)

	fmt.Println(rsp)

	rspData, err := json.Marshal(rsp)
	if err != nil {
		panic(err)
	}

	w.Write(rspData)

	fmt.Println("done")
}

// DeviceArch represents the features that the device support.
type DeviceArch struct {
	HasGlobalInt32Atomics    bool `json:"has_global_int_32_atomics"`
	HasGlobalFloatAtomicExch bool `json:"has_global_float_atomic_exch"`
	HasSharedInt32Atomics    bool `json:"has_shared_int_32_atomics"`
	HasSharedFloatAtomicExch bool `json:"has_shared_float_atomic_exch"`
	HasFloatAtomicAdd        bool `json:"has_float_atomic_add"`
	HasGlobalInt64Atomics    bool `json:"has_global_int_64_atomics"`
	HasSharedInt64Atomics    bool `json:"has_shared_int_64_atomics"`
	HasDoubles               bool `json:"has_doubles"`
	HasWarpVote              bool `json:"has_warp_vote"`
	HasWarpBallot            bool `json:"has_warp_ballot"`
	HasWarpShuffle           bool `json:"has_warp_shuffle"`
	HasFunnelShift           bool `json:"has_funnel_shift"`
	HasThreadFenceSystem     bool `json:"has_thread_fence_system"`
	HasSyncThreadsExt        bool `json:"has_sync_threads_ext"`
	HasSurfaceFuncs          bool `json:"has_surface_funcs"`
	Has3dGrid                bool `json:"has_3d_grid"`
	HasDynamicParalleli      bool `json:"has_dynamic_paralleli"`
}

// The DeviceProperty is the message sent back to the host when querying device properties.
type DeviceProperty struct {
	Name                             string     `json:"name"`
	TotalGlobalMem                   uint64     `json:"total_global_mem"`
	SharedMemPerBlock                uint64     `json:"shared_mem_per_block"`
	RegsPerBlock                     int        `json:"regs_per_block"`
	WarpSize                         int        `json:"warp_size"`
	MaxThreadsPerBlock               int        `json:"max_threads_per_block"`
	MaxThreadsDim                    [3]int     `json:"max_threads_dim"`
	MaxGridSize                      [3]int     `json:"max_grid_size"`
	ClockRate                        int        `json:"clock_rate"`
	MemClockRate                     int        `json:"mem_clock_rate"`
	MemoryBusWidth                   int        `json:"memory_bus_width"`
	TotalConstMem                    int        `json:"total_const_mem"`
	Major                            int        `json:"major"`
	Minor                            int        `json:"minor"`
	MultiProcessorCount              int        `json:"multi_processor_count"`
	L2CacheSize                      uint64     `json:"l_2_cache_size"`
	MaxThreadsPerMultiProcessor      int        `json:"max_threads_per_multi_processor"`
	ComputeMode                      int        `json:"compute_mode"`
	ClockInstructionRate             int        `json:"clock_instruction_rate"`
	Arch                             DeviceArch `json:"arch"`
	ConcurrentKernels                int        `json:"concurrent_kernels"`
	PCIBusID                         int        `json:"pci_bus_id"`
	PCIDeviceID                      int        `json:"pci_device_id"`
	MaxSharedMemoryPerMultiProcessor int        `json:"max_shared_memory_per_multi_processor"`
	IsMultiGPUBoard                  int        `json:"is_multi_gpu_board"`
	CanMapHostMemory                 int        `json:"can_map_host_memory"`
	GCNArch                          int        `json:"gcn_arch"`
}

func handleDeviceProperties(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := mux.Vars(r)["id"]

	deviceID, err := strconv.Atoi(deviceIDStr)
	if err != nil {
		http.Error(w, err.Error(), 400)
	}

	if deviceID < 0 {
		http.Error(w, "device id must be a positive number", 400)
	}

	if deviceID > len(serverInstance.driver.GPUs) {
		http.Error(w, "GPU does not exist", 404)
	}

	deviceProperty := getDeviceProperty(deviceID)

	rspData, err := json.Marshal(deviceProperty)
	if err != nil {
		panic(err)
	}

	w.Write(rspData)
}

func getDeviceProperty(deviceID int) DeviceProperty {
	// gpu := serverInstance.driver.GPUs[deviceID-1]
	dp := DeviceProperty{
		Name:                             "gfx803",
		TotalGlobalMem:                   0,
		SharedMemPerBlock:                4096,
		RegsPerBlock:                     102,
		WarpSize:                         64,
		MaxThreadsPerBlock:               1024,
		MaxThreadsDim:                    [3]int{1024, 256, 64},
		MaxGridSize:                      [3]int{1048576, 65536, 1024},
		ClockRate:                        1048576,
		MemClockRate:                     1048576,
		MemoryBusWidth:                   4096,
		TotalConstMem:                    1048576,
		Major:                            8,
		Minor:                            3,
		MultiProcessorCount:              64,
		L2CacheSize:                      0,
		MaxThreadsPerMultiProcessor:      2560,
		ComputeMode:                      1,
		ClockInstructionRate:             1048576,
		Arch:                             DeviceArch{},
		ConcurrentKernels:                1,
		PCIBusID:                         0,
		PCIDeviceID:                      0,
		MaxSharedMemoryPerMultiProcessor: 65536,
		IsMultiGPUBoard:                  0,
		CanMapHostMemory:                 0,
		GCNArch:                          803,
	}
	return dp
}
