package driver

import "gitlab.com/akita/mem/vm"

type Context struct {
	PID          vm.PID
	CurrentGPUID int
}
