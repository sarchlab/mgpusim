package cachelatency

import "math"

// shuffledOrdinals reproduces libstdc++ std::shuffle with std::mt19937.
// libstdc++ draws a pair of swap indices together when both fit in one draw.
func shuffledOrdinals(size int, seed uint32) []uint32 {
	order := make([]uint32, size)
	for i := range order {
		order[i] = uint32(i)
	}
	rng := newMT19937(seed)
	if size < 2 {
		return order
	}
	if math.MaxUint32/size >= size {
		i := 1
		if size%2 == 0 {
			j := rng.below(2)
			order[i], order[j] = order[j], order[i]
			i++
		}
		for ; i < size; i += 2 {
			nextRange := uint32(i + 2)
			pair := rng.below(uint32(i+1) * nextRange)
			j, k := pair/nextRange, pair%nextRange
			order[i], order[j] = order[j], order[i]
			order[i+1], order[k] = order[k], order[i+1]
		}
		return order
	}
	for i := 1; i < size; i++ {
		j := rng.below(uint32(i + 1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

type mt19937 struct {
	state [624]uint32
	index int
}

func newMT19937(seed uint32) *mt19937 {
	rng := &mt19937{index: 624}
	rng.state[0] = seed
	for i := 1; i < len(rng.state); i++ {
		prev := rng.state[i-1]
		rng.state[i] = 1812433253*(prev^(prev>>30)) + uint32(i)
	}
	return rng
}

func (rng *mt19937) next() uint32 {
	if rng.index == len(rng.state) {
		for i := range rng.state {
			next := (i + 1) % len(rng.state)
			mixed := rng.state[i]&0x80000000 | rng.state[next]&0x7fffffff
			value := rng.state[(i+397)%len(rng.state)] ^ (mixed >> 1)
			if mixed&1 != 0 {
				value ^= 0x9908b0df
			}
			rng.state[i] = value
		}
		rng.index = 0
	}
	value := rng.state[rng.index]
	rng.index++
	value ^= value >> 11
	value ^= (value << 7) & 0x9d2c5680
	value ^= (value << 15) & 0xefc60000
	return value ^ (value >> 18)
}

// below reproduces the libstdc++ uniform_int_distribution multiply/reject
// path for an engine with a 32-bit output range.
func (rng *mt19937) below(limit uint32) uint32 {
	threshold := -limit % limit
	for {
		product := uint64(rng.next()) * uint64(limit)
		if uint32(product) >= threshold {
			return uint32(product >> 32)
		}
	}
}
