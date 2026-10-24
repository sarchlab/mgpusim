package kernels

// WGSelection selects a contiguous, half-open range of flattened work-group
// indices. The zero value selects the entire grid. Partitioned distinguishes
// an empty partition from the entire grid without an executable callback.
type WGSelection struct {
	Partitioned bool   `json:"partitioned"`
	First       uint64 `json:"first"`
	End         uint64 `json:"end"`
}

// Includes reports whether the flattened work-group index belongs to this
// selection. X varies fastest, followed by Y and Z.
func (s WGSelection) Includes(index uint64) bool {
	return !s.Partitioned || (index >= s.First && index < s.End)
}

// Count returns the number of selected work-groups in a grid of size total.
func (s WGSelection) Count(total uint64) uint64 {
	if !s.Partitioned {
		return total
	}
	end := s.End
	if end > total {
		end = total
	}
	if s.First >= end {
		return 0
	}
	return end - s.First
}
