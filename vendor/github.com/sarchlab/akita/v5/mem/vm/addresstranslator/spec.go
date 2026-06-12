package addresstranslator

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	Freq           sim.Freq `json:"freq"`
	Log2PageSize   uint64   `json:"log2_page_size"`
	DeviceID       uint64   `json:"device_id"`
	NumReqPerCycle int      `json:"num_req_per_cycle"`

	MemMapperKind             string           `json:"mem_mapper_kind"`
	MemMapperPorts            []sim.RemotePort `json:"mem_mapper_ports"`
	MemMapperInterleavingSize uint64           `json:"mem_mapper_interleaving_size"`

	TransMapperKind             string           `json:"trans_mapper_kind"`
	TransMapperPorts            []sim.RemotePort `json:"trans_mapper_ports"`
	TransMapperInterleavingSize uint64           `json:"trans_mapper_interleaving_size"`
}

// findMemoryPort implements the same Find() logic as SinglePortMapper and
// InterleavedAddressPortMapper, using Spec fields.
func findMemoryPort(spec Spec, addr uint64) sim.RemotePort {
	switch spec.MemMapperKind {
	case "single":
		return spec.MemMapperPorts[0]
	case "interleaved":
		n := addr / spec.MemMapperInterleavingSize %
			uint64(len(spec.MemMapperPorts))
		return spec.MemMapperPorts[n]
	default:
		panic("invalid mem mapper kind: " + spec.MemMapperKind)
	}
}

// findTranslationPort implements the same Find() logic as SinglePortMapper and
// InterleavedAddressPortMapper, using Spec fields.
func findTranslationPort(spec Spec, addr uint64) sim.RemotePort {
	switch spec.TransMapperKind {
	case "single":
		return spec.TransMapperPorts[0]
	case "interleaved":
		n := addr / spec.TransMapperInterleavingSize %
			uint64(len(spec.TransMapperPorts))
		return spec.TransMapperPorts[n]
	default:
		panic("invalid trans mapper kind: " + spec.TransMapperKind)
	}
}
