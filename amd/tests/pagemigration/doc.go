// Package pagemigration holds the acceptance tests of page migration between
// GPUs. The tests build a complete timing platform with several GPUs, an
// R9 Nano one and an MI300X one, run real kernels on unified memory, and
// check the contents of the pages and the statistics of the driver. One
// test, in writeback_test.go, builds only the path of a migrating page from
// the components of the platforms, so that it can hold a writeback of the L2
// back. See docs/page-migration.md.
//
// The tests take minutes; they skip with -short, and the unit-test job skips
// the package:
//
//	go test ./amd/tests/pagemigration/ -count=1 -timeout 60m
package pagemigration
