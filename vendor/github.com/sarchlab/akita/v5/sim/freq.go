package sim

import "log"

// Freq defines the type of frequency in Hz.
type Freq uint64

// Defines the unit of frequency.
const (
	Hz  Freq = 1
	KHz Freq = 1e3
	MHz Freq = 1e6
	GHz Freq = 1e9
)

// psPerSecond is the number of picoseconds in one second.
const psPerSecond uint64 = 1_000_000_000_000

// Period returns the time between two consecutive ticks in picoseconds.
func (f Freq) Period() VTimeInSec {
	if f == 0 {
		log.Panic("frequency cannot be 0")
	}
	return VTimeInSec(psPerSecond / uint64(f))
}

// Cycle converts a time to the number of cycles passed since time 0.
func (f Freq) Cycle(time VTimeInSec) uint64 {
	return uint64(time) / (psPerSecond / uint64(f))
}

// ThisTick returns the current tick time — ceil to nearest tick boundary.
func (f Freq) ThisTick(now VTimeInSec) VTimeInSec {
	period := uint64(f.Period())
	n := uint64(now)
	return VTimeInSec(((n + period - 1) / period) * period)
}

// NextTick returns the next tick time after now.
func (f Freq) NextTick(now VTimeInSec) VTimeInSec {
	period := uint64(f.Period())
	n := uint64(now)
	return VTimeInSec((n/period + 1) * period)
}

// NCyclesLater returns the time after N cycles from the current tick.
func (f Freq) NCyclesLater(n int, now VTimeInSec) VTimeInSec {
	period := uint64(f.Period())
	base := uint64(f.ThisTick(now))
	return VTimeInSec(base + uint64(n)*period)
}

// NoEarlierThan returns the tick time that is at or right after the given time.
// This is equivalent to ThisTick.
func (f Freq) NoEarlierThan(t VTimeInSec) VTimeInSec {
	return f.ThisTick(t)
}

// HalfTick returns the time in middle of two ticks.
func (f Freq) HalfTick(t VTimeInSec) VTimeInSec {
	return f.ThisTick(t) + f.Period()/2
}
