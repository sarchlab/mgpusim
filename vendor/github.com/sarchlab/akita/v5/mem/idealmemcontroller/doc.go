// Package idealmemcontroller provides an implementation of an ideal memory
// controller, which has a fixed latency and unlimited concurrency.
package idealmemcontroller

//go:generate mockgen -destination mock_sim_test.go -package idealmemcontroller -write_package_comment=false github.com/sarchlab/akita/v5/sim Engine,Port
