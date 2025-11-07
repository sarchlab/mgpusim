package smsp

import (
	"log"
)

// type ResourcePool struct {
// 	IntUnits     int
// 	FP32Units    int
// 	FP64Units    int
// 	TensorUnits  int
// 	LdStUnits    int
// 	SpecialUnits int
// }

type ResourcePool struct {
	IntUnitPool     bool
	FP32UnitPool    bool
	FP64UnitPool    bool
	TensorUnitPool  bool
	LdStUnitPool    bool
	SpecialUnitPool bool
}

// Create H100 SMSP resource model
// func NewH100SMSPResourcePool() *ResourcePool {
// 	return &ResourcePool{
// 		IntUnits:     16,
// 		FP32Units:    32,
// 		FP64Units:    16,
// 		TensorUnits:  8,
// 		LdStUnits:    4,
// 		SpecialUnits: 4,
// 	}
// }

func NewH100SMSPResourcePool() *ResourcePool {
	return &ResourcePool{
		IntUnitPool:     true,
		FP32UnitPool:    true,
		FP64UnitPool:    true,
		TensorUnitPool:  true,
		LdStUnitPool:    true,
		SpecialUnitPool: true,
	}
}

func (rp *ResourcePool) Reserve(unit ExecUnitKind) bool {
	switch unit {
	case UnitNone:
		return true
	case UnitInt:
		if !rp.IntUnitPool {
			return false
		}
		rp.IntUnitPool = false
	case UnitFP32:
		if !rp.FP32UnitPool {
			return false
		}
		rp.FP32UnitPool = false
	case UnitFP64:
		if !rp.FP64UnitPool {
			return false
		}
		rp.FP64UnitPool = false
	case UnitTensor:
		if !rp.TensorUnitPool {
			return false
		}
		rp.TensorUnitPool = false
	case UnitLdSt:
		if !rp.LdStUnitPool {
			return false
		}
		rp.LdStUnitPool = false
	case UnitSpecial:
		if !rp.SpecialUnitPool {
			return false
		}
		rp.SpecialUnitPool = false
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
		rp.IntUnitPool = true
	case UnitFP32:
		rp.FP32UnitPool = true
	case UnitFP64:
		rp.FP64UnitPool = true
	case UnitTensor:
		rp.TensorUnitPool = true
	case UnitLdSt:
		rp.LdStUnitPool = true
	case UnitSpecial:
		rp.SpecialUnitPool = true
	default:
		log.Panic("Release: Unknown execution unit type:", unit)
	}
}

// Try to reserve units; return success
// func (rp *ResourcePool) Reserve(unit ExecUnitKind, count int) bool {
// 	// fmt.Printf("Trying to reserve %d units of type %d: (%d, %d, %d, %d, %d, %d)\n",
// 	// 	count, unit,
// 	// 	rp.IntUnits, rp.FP32Units, rp.FP64Units, rp.TensorUnits, rp.LdStUnits, rp.SpecialUnits)
// 	// before := fmt.Sprintf("(%d, %d, %d, %d, %d, %d)", rp.IntUnits, rp.FP32Units, rp.FP64Units, rp.TensorUnits, rp.LdStUnits, rp.SpecialUnits)
// 	switch unit {
// 	case UnitInt:
// 		if rp.IntUnits < count {
// 			return false
// 		}
// 		rp.IntUnits -= count
// 	case UnitFP32:
// 		if rp.FP32Units < count {
// 			return false
// 		}
// 		rp.FP32Units -= count
// 	case UnitFP64:
// 		if rp.FP64Units < count {
// 			return false
// 		}
// 		rp.FP64Units -= count
// 	case UnitTensor:
// 		if rp.TensorUnits < count {
// 			return false
// 		}
// 		rp.TensorUnits -= count
// 	case UnitLdSt:
// 		if rp.LdStUnits < count {
// 			return false
// 		}
// 		rp.LdStUnits -= count
// 	case UnitSpecial:
// 		if rp.SpecialUnits < count {
// 			return false
// 		}
// 		rp.SpecialUnits -= count
// 	default:
// 		log.Panic("Reserve: Unknown execution unit type")
// 	}
// 	// fmt.Printf("Reserved %d units of type %d: %s -> (%d, %d, %d, %d, %d, %d)\n",
// 	// 	count, unit,
// 	// 	before,
// 	// 	rp.IntUnits, rp.FP32Units, rp.FP64Units, rp.TensorUnits, rp.LdStUnits, rp.SpecialUnits)
// 	return true
// }

// func (rp *ResourcePool) Release(unit ExecUnitKind, count int) {
// 	// before := fmt.Sprintf("(%d, %d, %d, %d, %d, %d)", rp.IntUnits, rp.FP32Units, rp.FP64Units, rp.TensorUnits, rp.LdStUnits, rp.SpecialUnits)
// 	switch unit {
// 	case UnitInt:
// 		rp.IntUnits += count
// 	case UnitFP32:
// 		rp.FP32Units += count
// 	case UnitFP64:
// 		rp.FP64Units += count
// 	case UnitTensor:
// 		rp.TensorUnits += count
// 	case UnitLdSt:
// 		rp.LdStUnits += count
// 	case UnitSpecial:
// 		rp.SpecialUnits += count
// 	default:
// 		log.Panic("Release: Unknown execution unit type")
// 	}
// 	// fmt.Printf("Released %d units of type %d: %s -> (%d, %d, %d, %d, %d, %d)\n",
// 	// 	count, unit,
// 	// 	before,
// 	// 	rp.IntUnits, rp.FP32Units, rp.FP64Units, rp.TensorUnits, rp.LdStUnits, rp.SpecialUnits)
// }
