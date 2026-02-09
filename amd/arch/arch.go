// Package arch provides architecture configurations for AMD GPU architectures.
package arch

// Type represents the GPU architecture type.
type Type int

const (
	// GCN3 represents the GCN3 architecture (gfx803).
	GCN3 Type = iota
	// CDNA3 represents the CDNA3 architecture (gfx942).
	CDNA3
)

// String returns the string representation of the architecture type.
func (t Type) String() string {
	switch t {
	case GCN3:
		return "GCN3"
	case CDNA3:
		return "CDNA3"
	default:
		return "Unknown"
	}
}

// Config represents architecture-specific configuration.
type Config struct {
	// Name is the human-readable architecture name.
	Name string

	// Type is the architecture type.
	Type Type

	// NumVGPRsPerLane is the number of VGPRs per work-item.
	NumVGPRsPerLane int

	// NumSGPRs is the number of SGPRs per wavefront.
	NumSGPRs int

	// WavefrontSize is the number of work-items per wavefront.
	WavefrontSize int

	// LDSSize is the size of the Local Data Share in bytes.
	LDSSize int
}
