package driver

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/vis/trace"
)

var _ = ginkgo.Describe("KernelTimeCounter", func() {
	ginkgo.It("should count kernel time", func() {
		c := NewKernelTimeCounter()

		task1 := trace.Task{ID: "1", What: "*driver.LaunchKernelCommand"}
		ctx1 := &akita.HookCtx{
			Now:  akita.VTimeInSec(0.5),
			Pos:  trace.HookPosTaskInitiate,
			Item: task1,
		}
		c.Func(ctx1)

		task2 := trace.Task{ID: "2", What: "*driver.LaunchKernelCommand"}
		ctx2 := &akita.HookCtx{
			Now:  akita.VTimeInSec(1.0),
			Pos:  trace.HookPosTaskInitiate,
			Item: task2,
		}
		c.Func(ctx2)

		task3 := trace.Task{ID: "3", What: "*driver.LaunchKernelCommand"}
		ctx3 := &akita.HookCtx{
			Now:  akita.VTimeInSec(1.3),
			Pos:  trace.HookPosTaskInitiate,
			Item: task3,
		}
		c.Func(ctx3)

		task1e := trace.Task{ID: "1"}
		ctx1e := &akita.HookCtx{
			Now:  akita.VTimeInSec(1.5),
			Pos:  trace.HookPosTaskClear,
			Item: task1e,
		}
		c.Func(ctx1e)

		task3e := trace.Task{ID: "3"}
		ctx3e := &akita.HookCtx{
			Now:  akita.VTimeInSec(1.7),
			Pos:  trace.HookPosTaskClear,
			Item: task3e,
		}
		c.Func(ctx3e)

		task2e := trace.Task{ID: "2"}
		ctx2e := &akita.HookCtx{
			Now:  akita.VTimeInSec(2.0),
			Pos:  trace.HookPosTaskClear,
			Item: task2e,
		}
		c.Func(ctx2e)

		task4 := trace.Task{ID: "4", What: "*driver.LaunchKernelCommand"}
		ctx4 := &akita.HookCtx{
			Now:  akita.VTimeInSec(3.0),
			Pos:  trace.HookPosTaskInitiate,
			Item: task4,
		}
		c.Func(ctx4)

		task4e := trace.Task{ID: "4"}
		ctx4e := &akita.HookCtx{
			Now:  akita.VTimeInSec(3.5),
			Pos:  trace.HookPosTaskClear,
			Item: task4e,
		}
		c.Func(ctx4e)

		Expect(c.TotalTime).To(Equal(akita.VTimeInSec(2.0)))

	})
})
