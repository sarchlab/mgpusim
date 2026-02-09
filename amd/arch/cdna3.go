package arch

// CDNA3Config provides the configuration for CDNA3 (gfx942) architecture.
var CDNA3Config = &Config{
	Name:            "CDNA3",
	Type:            CDNA3,
	NumVGPRsPerLane: 512,
	NumSGPRs:        106,
	WavefrontSize:   64,
	LDSSize:         65536, // 64KB
}
