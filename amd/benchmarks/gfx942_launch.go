package benchmarks

// GFX942LaunchDimensions is the leading part of the AMDGPU code object
// implicit kernel arguments. Put it at the first hidden argument offset
// reported in the code object note, after the explicit arguments.
type GFX942LaunchDimensions struct {
	BlockCount [3]uint32
	GroupSize  [3]uint16
	Remainder  [3]uint16
}

// NewGFX942LaunchDimensions encodes grid and work-group dimensions for HIP
// kernels that read the implicit arguments.
func NewGFX942LaunchDimensions(grid [3]uint32, group [3]uint16) GFX942LaunchDimensions {
	result := GFX942LaunchDimensions{GroupSize: group}
	for d, n := range group {
		result.BlockCount[d] = grid[d] / uint32(n)
		result.Remainder[d] = uint16(grid[d] % uint32(n))
	}
	return result
}
