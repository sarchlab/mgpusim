package arbitration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Buffer
func TestArbitration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Arbitration Suite")
}
