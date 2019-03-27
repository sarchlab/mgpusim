package driver

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
)

var _ = ginkgo.Describe("KernelTimeCounter", func(){
	ginkgo.It("should count kernel time", func() {
		c := NewKernelTimeCounter()

		cmd1 := &LaunchKernelCommand{}
		info1 := &CommandHookInfo{Now: akita.VTimeInSec(0.5), IsStart:true}
		c.Func(cmd1, nil, info1)

		cmd2 := &LaunchKernelCommand{}
		info2 := &CommandHookInfo{Now: akita.VTimeInSec(1.0), IsStart:true}
		c.Func(cmd2, nil, info2)

		cmd3 := &LaunchKernelCommand{}
		info3 := &CommandHookInfo{Now: akita.VTimeInSec(1.3), IsStart:true}
		c.Func(cmd3, nil, info3)

		info1e := &CommandHookInfo{Now: akita.VTimeInSec(1.5), IsStart:false}
		c.Func(cmd1, nil, info1e)

		info3e := &CommandHookInfo{Now: akita.VTimeInSec(1.7), IsStart:false}
		c.Func(cmd3, nil, info3e)

		info2e := &CommandHookInfo{Now: akita.VTimeInSec(2.0), IsStart:false}
		c.Func(cmd2, nil, info2e)

		cmd4 := &LaunchKernelCommand{}
		info4 := &CommandHookInfo{Now: akita.VTimeInSec(3.0), IsStart:true}
		c.Func(cmd4, nil, info4)

		info4e := &CommandHookInfo{Now: akita.VTimeInSec(3.5), IsStart:false}
		c.Func(cmd4, nil, info4e)

		Expect(c.TotalTime).To(Equal(akita.VTimeInSec(2.0)))

	})
})