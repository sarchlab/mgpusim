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

type dataJSON struct {
	Data []float64
	Size []int
}

type inputOutputPair struct {
	Input  dataJSON
	Output dataJSON
}

func loadInputOutputPair(filename string) []inputOutputPair {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var pairs []inputOutputPair

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &pairs)
	if err != nil {
		panic(err)
	}

	return pairs
}
