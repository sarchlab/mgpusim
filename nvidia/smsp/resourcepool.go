package smsp

type ResourcePool struct {
	IntUnits     int
	FP32Units    int
	FP64Units    int
	TensorUnits  int
	LdStUnits    int
	SpecialUnits int
}

// Create H100 SMSP resource model
func NewH100SMSPResourcePool() *ResourcePool {
	return &ResourcePool{
		IntUnits:     16,
		FP32Units:    32,
		FP64Units:    16,
		TensorUnits:  8,
		LdStUnits:    4,
		SpecialUnits: 4,
	}
}

// Try to reserve units; return success
func (rp *ResourcePool) Reserve(unit ExecUnitKind, count int) bool {
	switch unit {
	case UnitInt:
		if rp.IntUnits < count {
			return false
		}
		rp.IntUnits -= count
	case UnitFP32:
		if rp.FP32Units < count {
			return false
		}
		rp.FP32Units -= count
	case UnitFP64:
		if rp.FP64Units < count {
			return false
		}
		rp.FP64Units -= count
	case UnitTensor:
		if rp.TensorUnits < count {
			return false
		}
		rp.TensorUnits -= count
	case UnitLdSt:
		if rp.LdStUnits < count {
			return false
		}
		rp.LdStUnits -= count
	}
	return true
}

func (rp *ResourcePool) Release(unit ExecUnitKind, count int) {
	switch unit {
	case UnitInt:
		rp.IntUnits += count
	case UnitFP32:
		rp.FP32Units += count
	case UnitFP64:
		rp.FP64Units += count
	case UnitTensor:
		rp.TensorUnits += count
	case UnitLdSt:
		rp.LdStUnits += count
	}
}
