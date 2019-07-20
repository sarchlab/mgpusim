package timing

import "gitlab.com/akita/gcn3/timing/wavefront"

//go:generate mockgen -source=$GOFILE -destination=mock_coalescer_test.go -self_package="gitlab.com/akita/gcn3/timing" -package $GOPACKAGE -write_package_comment=false

// Coalescer can generate memory access instructions from instruction, register
// values.
type coalescer interface {
	generateMemTransactions(wf *wavefront.Wavefront) []VectorMemAccessInfo
}
