package cu

import (
	"math/bits"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// An indexed gfx942 LDS access uses 32 dword banks. Each phase completes
// after its busiest bank has serviced all of its words. The DS_READ_B128
// lane groups come from the MI300X empirical LDS instruction measurements:
// https://github.com/nod-ai/amd-shark-ai/blob/efa471aeef66a260c85983cc41e833bfa769dade/docs/empirical-lds/cdna3info.md
// We use the corresponding DS phase schedule for shared FLAT, and the
// four-dword schedule for three-dword lanes.
func ldsBankRounds(access insts.CDNA3LDSAccess, exec uint64, bases *[64]uint32) (rounds, replays int) {
	lanes := 32
	if access.Bytes > 8 {
		lanes = 8
	} else if access.Bytes > 4 {
		lanes = 16
	}
	for _, offset := range access.Offsets[:access.Count] {
		for first := 0; first < 64; first += lanes {
			mask := (uint64(1)<<lanes - 1) << first
			if lanes == 8 && !access.Write {
				// Quads within a half-wave are paired as (0,5), (1,4),
				// (2,7), (3,6), as distinct from contiguous store phases.
				quad := (first/32)*8 + (first%32)/8
				mask = 15<<(quad*4) | 15<<((quad^5)*4)
			}
			n := ldsPhaseRounds(access, exec&mask, bases, offset)
			rounds += n
			replays += max(0, n-1)
		}
	}
	return rounds, replays
}

func ldsPhaseRounds(access insts.CDNA3LDSAccess, active uint64, bases *[64]uint32, offset uint32) int {
	var banks [32]int
	seen := make(map[uint32]bool)
	rounds := 0
	for ; active != 0; active &= active - 1 {
		address := uint64(bases[bits.TrailingZeros64(active)] + offset)
		for word := uint32(address / 4); word <= uint32((address+uint64(access.Bytes)-1)/4); word++ {
			bank := word % 32
			if !access.Write {
				if seen[word] {
					continue // broadcast within this phase
				}
				seen[word] = true
			}
			banks[bank]++
			rounds = max(rounds, banks[bank])
		}
	}
	return rounds
}
