package emu

import "gitlab.com/yaotsu/gcn3"

// A InstWorker is where one instruction got executed
type InstWorker struct {
	CU *gcn3.ComputeUnit
}
