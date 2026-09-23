package arch

// GCN3Config provides the configuration for GCN3 (gfx803) architecture.
var GCN3Config = &Config{
	Name:            "GCN3",
	Type:            GCN3,
	NumVGPRsPerLane: 256,
	NumSGPRs:        102,
	WavefrontSize:   64,
	LDSSize:         65536, // 64KB
}
