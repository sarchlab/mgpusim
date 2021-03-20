package rob

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -write_package_comment=false -package=$GOPACKAGE -destination=mock_sim_test.go gitlab.com/akita/akita/v2/sim Port,Engine

func TestRob(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rob Suite")
}
