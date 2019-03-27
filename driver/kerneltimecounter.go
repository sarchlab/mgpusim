package driver

import (
	"reflect"

	"gitlab.com/akita/akita"
)

type kernelStartEnd struct {
	start, end akita.VTimeInSec
}

// KernelTimeCounter is a hook to the driver that can calculate the
// amount of time spent on kernel execution.
type KernelTimeCounter struct {
	TotalTime akita.VTimeInSec

	startTimes  map[*LaunchKernelCommand]akita.VTimeInSec
	kernelTimes []*kernelStartEnd
}

func NewKernelTimeCounter() *KernelTimeCounter {
	c := new(KernelTimeCounter)
	c.startTimes = make(map[*LaunchKernelCommand]akita.VTimeInSec)
	return c
}

// Type specifies the type it hooks to.
func (c *KernelTimeCounter) Type() reflect.Type {
	return reflect.TypeOf((*LaunchKernelCommand)(nil))
}

// Pos specifies the position it hooks to.
func (c *KernelTimeCounter) Pos() akita.HookPos {
	return akita.AnyHookPos
}

// Func calculates the time spent on kernel execution.
func (c *KernelTimeCounter) Func(
	item interface{},
	domain akita.Hookable,
	info interface{},
) {
	cmd := item.(*LaunchKernelCommand)
	hookInfo := info.(*CommandHookInfo)

	if hookInfo.IsStart {
		c.startTimes[cmd] = hookInfo.Now
		return
	}

	startTime := c.startTimes[cmd]
	kernelTime := &kernelStartEnd{
		start: startTime,
		end:   hookInfo.Now,
	}
	c.kernelTimes = append(c.kernelTimes, kernelTime)

	c.updateTotalTime()
}

func (c *KernelTimeCounter) updateTotalTime() {
	c.TotalTime = 0
	coveredMask := make(map[int]bool)

	for i, t := range c.kernelTimes {
		if _, covered := coveredMask[i]; covered {
			continue
		}

		coveredMask[i] = true

		extTime := &kernelStartEnd{
			start: t.start,
			end:   t.end,
		}

		for j, t2 := range c.kernelTimes {
			if _, covered := coveredMask[j]; covered {
				continue
			}

			if kernelTimeOverlap(t, t2) {
				coveredMask[j] = true
				extendKernelTime(extTime, t2)
			}
		}

		c.TotalTime += extTime.end - extTime.start
	}
}

func kernelTimeOverlap(t1, t2 *kernelStartEnd) bool {
	if t1.start <= t2.start && t1.end >= t2.start {
		return true
	}

	if t1.start <= t2.end && t1.end >= t2.end {
		return true
	}

	if t1.start >= t2.start && t1.end <= t2.end {
		return true
	}

	return false
}

func extendKernelTime(base, t2 *kernelStartEnd) {
	if t2.start < base.start {
		base.start = t2.start
	}

	if t2.end > base.end {
		base.end = t2.end
	}
}
