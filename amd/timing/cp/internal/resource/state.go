package resource

import "github.com/sarchlab/akita/v5/sim/messaging"

// CUConfig describes one CU's immutable dispatch destination and capacities.
type CUConfig struct {
	Port           messaging.RemotePort `json:"port"`
	WavefrontPools []int                `json:"wavefront_pools"`
	ScalarCount    int                  `json:"scalar_count"`
	VectorCounts   []int                `json:"vector_counts"`
	LDSCapacity    int                  `json:"lds_bytes"`
}

func (c CUConfig) DispatchingPort() messaging.RemotePort { return c.Port }
func (c CUConfig) WfPoolSizes() []int                    { return append([]int(nil), c.WavefrontPools...) }
func (c CUConfig) VRegCounts() []int                     { return append([]int(nil), c.VectorCounts...) }
func (c CUConfig) SRegCount() int                        { return c.ScalarCount }
func (c CUConfig) LDSBytes() int                         { return c.LDSCapacity }

// DescribeCU copies the topology and capacities from a CU during assembly.
func DescribeCU(c DispatchableCU) CUConfig {
	return CUConfig{Port: c.DispatchingPort(),
		WavefrontPools: append([]int(nil), c.WfPoolSizes()...),
		ScalarCount:    c.SRegCount(),
		VectorCounts:   append([]int(nil), c.VRegCounts()...),
		LDSCapacity:    c.LDSBytes()}
}

// CUState is the sole owner of occupancy and work-group reservations.
type CUState struct {
	FreeWavefronts []int
	Scalar, LDS    Mask
	Vector         []Mask
	NextSIMD       int
	Reserved       map[uint64][]WfLocation
}

// Bind builds immutable views that resolve the current component State on each access.
func Bind(config []CUConfig, state func() *[]CUState) *CUResourcePoolImpl {
	p := &CUResourcePoolImpl{}
	for i, c := range config {
		index := i
		p.cus = append(p.cus, &CUResourceImpl{config: c, state: func() *CUState { return &(*state())[index] }})
	}
	return p
}

// InitialState allocates reservation data once during component initialization.
func InitialState(config []CUConfig) []CUState {
	p := NewCUResourcePool()
	seen := make(map[messaging.RemotePort]bool)
	for _, c := range config {
		if c.Port == "" || seen[c.Port] {
			panic("cp: CU destinations must be nonempty and unique")
		}
		seen[c.Port] = true
		if len(c.WavefrontPools) == 0 || len(c.WavefrontPools) != len(c.VectorCounts) {
			panic("cp: CU wavefront and vector capacities must have equal positive SIMD counts")
		}
		p.RegisterCU(c)
	}
	out := make([]CUState, len(p.cus))
	for i, c := range p.cus {
		out[i] = *c.(*CUResourceImpl).state()
	}
	return out
}
