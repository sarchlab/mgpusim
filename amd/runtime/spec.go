package runtime

type DeviceProperties struct {
	CUCount  int    `json:"cu_count"`
	DRAMSize uint64 `json:"dram_size"`
}

// RuntimeSpec owns allocation and device properties independently of Driver timing.
type RuntimeSpec struct {
	Log2PageSize uint64             `json:"log2_page_size"`
	Devices      []DeviceProperties `json:"devices"`
}

func DefaultRuntimeSpec() RuntimeSpec { return RuntimeSpec{Log2PageSize: 12} }
