package benchmarks

// GlibcRand reproduces the Linux benchmark hosts' srand/rand sequence
// without changing process-global random state.
type GlibcRand struct {
	state [31]uint32
	front int
	rear  int
}

// NewGlibcRand seeds the additive generator used by glibc rand.
func NewGlibcRand(seed uint32) *GlibcRand {
	if seed == 0 {
		seed = 1
	}
	rng := &GlibcRand{front: 3}
	word := int32(seed)
	rng.state[0] = uint32(word)
	for i := 1; i < len(rng.state); i++ {
		hi, lo := word/127773, word%127773
		word = 16807*lo - 2836*hi
		if word < 0 {
			word += 2147483647
		}
		rng.state[i] = uint32(word)
	}
	for i := 0; i < 10*len(rng.state); i++ {
		rng.Next()
	}
	return rng
}

// Next returns one value between zero and glibc RAND_MAX, inclusive.
func (rng *GlibcRand) Next() uint32 {
	value := rng.state[rng.front] + rng.state[rng.rear]
	rng.state[rng.front] = value
	rng.front = (rng.front + 1) % len(rng.state)
	rng.rear = (rng.rear + 1) % len(rng.state)
	return value >> 1
}
