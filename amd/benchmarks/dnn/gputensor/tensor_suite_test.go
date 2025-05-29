package gputensor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTensor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tensor Suite")
}

var _ = AfterSuite(func() {
	files, err := filepath.Glob("*.sqlite3")
	if err != nil {
		return
	}

	for _, f := range files {
		if err := os.Remove(f); err != nil {
			fmt.Printf("Error removing file %s: %v\n", f, err)
		}
	}
})
