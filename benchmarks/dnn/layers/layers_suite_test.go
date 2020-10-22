package layers

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLayers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Layers Suite")
}

type tensorJSON struct {
	Data       []float64
	Size       []int
	Descriptor string
}

func (t tensorJSON) numElement() int {
	product := 1
	for _, s := range t.Size {
		product *= s
	}
	return product
}

func loadDatasets(filename string) []map[string]tensorJSON {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var pairs []map[string]tensorJSON

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &pairs)
	if err != nil {
		panic(err)
	}

	return pairs
}
