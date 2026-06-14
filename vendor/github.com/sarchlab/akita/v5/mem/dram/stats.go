package dram

// RowBufferHitRate returns the row-buffer hit rate (0.0 to 1.0).
func RowBufferHitRate(s *State) float64 {
	total := s.RowBufferHits + s.RowBufferMisses
	if total == 0 {
		return 0
	}
	return float64(s.RowBufferHits) / float64(total)
}

// AverageReadLatency returns the average read latency in cycles.
func AverageReadLatency(s *State) float64 {
	if s.CompletedReads == 0 {
		return 0
	}
	return float64(s.TotalReadLatencyCycles) / float64(s.CompletedReads)
}

// AverageWriteLatency returns the average write latency in cycles.
func AverageWriteLatency(s *State) float64 {
	if s.CompletedWrites == 0 {
		return 0
	}
	return float64(s.TotalWriteLatencyCycles) / float64(s.CompletedWrites)
}

// ReadBandwidth returns the read bandwidth in bytes per cycle.
func ReadBandwidth(s *State) float64 {
	if s.TotalCycles == 0 {
		return 0
	}
	return float64(s.BytesRead) / float64(s.TotalCycles)
}

// WriteBandwidth returns the write bandwidth in bytes per cycle.
func WriteBandwidth(s *State) float64 {
	if s.TotalCycles == 0 {
		return 0
	}
	return float64(s.BytesWritten) / float64(s.TotalCycles)
}
