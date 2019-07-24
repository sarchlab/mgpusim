package pagerank

type csrMatrix struct {
	rowOffsets    []uint32
	columnNumbers []uint32
	values        []float32
}
