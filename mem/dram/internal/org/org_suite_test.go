package org

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -source bank.go -destination mock_bank_test.go -self_package github.com/sarchlab/mgpusim/v3/mem/dram/internal/org -package $GOPACKAGE

func TestOrg(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Org Suite")
}
