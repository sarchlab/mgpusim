package smsp

import (
	"fmt"
	"log"
)

type ResourcePool struct {
	IntUnitCapacity     int
	FP32UnitCapacity    int
	FP64UnitCapacity    int
	TensorUnitCapacity  int
	LdStUnitCapacity    int
	SpecialUnitCapacity int

	IntUnitsAvailable     int
	FP32UnitsAvailable    int
	FP64UnitsAvailable    int
	TensorUnitsAvailable  int
	LdStUnitsAvailable    int
	SpecialUnitsAvailable int
}

func NewH100SMSPResourcePool() *ResourcePool {
	rp := &ResourcePool{
		IntUnitCapacity:     16,
		FP32UnitCapacity:    32,
		FP64UnitCapacity:    16,
		TensorUnitCapacity:  8,
		LdStUnitCapacity:    4,
		SpecialUnitCapacity: 4,
	}
	rp.Reset()
	return rp
}

func (rp *ResourcePool) Reset() {
	rp.IntUnitsAvailable = rp.IntUnitCapacity
	rp.FP32UnitsAvailable = rp.FP32UnitCapacity
	rp.FP64UnitsAvailable = rp.FP64UnitCapacity
	rp.TensorUnitsAvailable = rp.TensorUnitCapacity
	rp.LdStUnitsAvailable = rp.LdStUnitCapacity
	rp.SpecialUnitsAvailable = rp.SpecialUnitCapacity
}

func (rp *ResourcePool) Reserve(unit ExecUnitKind) bool {
	switch unit {
	case UnitNone:
		return true
	case UnitInt:
		if rp.IntUnitsAvailable <= 0 {
			return false
		}
		rp.IntUnitsAvailable--
	case UnitFP32:
		if rp.FP32UnitsAvailable <= 0 {
			return false
		}
		rp.FP32UnitsAvailable--
	case UnitFP64:
		if rp.FP64UnitsAvailable <= 0 {
			return false
		}
		rp.FP64UnitsAvailable--
	case UnitTensor:
		if rp.TensorUnitsAvailable <= 0 {
			return false
		}
		rp.TensorUnitsAvailable--
	case UnitLdSt:
		if rp.LdStUnitsAvailable <= 0 {
			return false
		}
		rp.LdStUnitsAvailable--
	case UnitSpecial:
		if rp.SpecialUnitsAvailable <= 0 {
			return false
		}
		rp.SpecialUnitsAvailable--
	default:
		log.Panic("Reserve: Unknown execution unit type:", unit)
	}
	return true
}

func (rp *ResourcePool) Release(unit ExecUnitKind) {
	switch unit {
	case UnitNone:
		return
	case UnitInt:
		if rp.IntUnitsAvailable < rp.IntUnitCapacity {
			rp.IntUnitsAvailable++
		}
	case UnitFP32:
		if rp.FP32UnitsAvailable < rp.FP32UnitCapacity {
			rp.FP32UnitsAvailable++
		}
	case UnitFP64:
		if rp.FP64UnitsAvailable < rp.FP64UnitCapacity {
			rp.FP64UnitsAvailable++
		}
	case UnitTensor:
		if rp.TensorUnitsAvailable < rp.TensorUnitCapacity {
			rp.TensorUnitsAvailable++
		}
	case UnitLdSt:
		if rp.LdStUnitsAvailable < rp.LdStUnitCapacity {
			rp.LdStUnitsAvailable++
		}
	case UnitSpecial:
		if rp.SpecialUnitsAvailable < rp.SpecialUnitCapacity {
			rp.SpecialUnitsAvailable++
		}
	default:
		log.Panic("Release: Unknown execution unit type:", unit)
	}
}

func (rp *ResourcePool) String() string {
	return fmt.Sprintf(
		"Int %d/%d, FP32 %d/%d, FP64 %d/%d, Tensor %d/%d, LdSt %d/%d, Special %d/%d",
		rp.IntUnitsAvailable, rp.IntUnitCapacity,
		rp.FP32UnitsAvailable, rp.FP32UnitCapacity,
		rp.FP64UnitsAvailable, rp.FP64UnitCapacity,
		rp.TensorUnitsAvailable, rp.TensorUnitCapacity,
		rp.LdStUnitsAvailable, rp.LdStUnitCapacity,
		rp.SpecialUnitsAvailable, rp.SpecialUnitCapacity,
	)
}
