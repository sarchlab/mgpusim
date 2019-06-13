package gcn3

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/vis/trace"
)

type kernelStartEnd struct {
	start, end akita.VTimeInSec
}

// KernelTimeCounter is a hook to the driver that can calculate the
// amount of time spent on kernel execution.
type KernelTimeCounter struct {
	TotalTime akita.VTimeInSec

	startTimes  map[string]akita.VTimeInSec
	kernelTimes []*kernelStartEnd
}

// NewKernelTimeCounter returns a newly created KernelTimeCounter
func NewKernelTimeCounter() *KernelTimeCounter {
	c := new(KernelTimeCounter)
	c.startTimes = make(map[string]akita.VTimeInSec)
	return c
}

// Func calculates the time spent on kernel execution.
func (c *KernelTimeCounter) Func(ctx *akita.HookCtx) {
	switch ctx.Pos {
	case trace.HookPosTaskInitiate:
		task := ctx.Item.(trace.Task)
		if task.What == "*driver.LaunchKernelCommand" {
			c.startTimes[task.ID] = ctx.Now
		}
	case trace.HookPosTaskClear:
		task := ctx.Item.(trace.Task)
		startTime, ok := c.startTimes[task.ID]
		if !ok {
			return
		}
		kernelTime := &kernelStartEnd{
			start: startTime,
			end:   ctx.Now,
		}
		c.kernelTimes = append(c.kernelTimes, kernelTime)

		c.updateTotalTime()
	}
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
