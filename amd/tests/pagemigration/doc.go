// Package pagemigration holds the acceptance tests of page migration between
// GPUs. Every test builds a complete timing platform with several GPUs, an
// R9 Nano one and an MI300X one, runs real kernels on unified memory, and
// checks the contents of the pages and the statistics of the driver. See
// docs/page-migration.md.
//
// The tests take minutes; they skip with -short, and the unit-test job skips
// the package:
//
//	go test ./amd/tests/pagemigration/ -count=1 -timeout 60m
package pagemigration
