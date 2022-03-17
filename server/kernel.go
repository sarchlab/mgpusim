package server

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"gitlab.com/akita/mgpusim/v3/insts"
)

type dim3 struct {
	X, Y, Z int
}

type launchKernelInput struct {
	CodeObject     string `json:"code_object,omitempty"`
	Args           string `json:"args,omitempty"`
	NumBlocks      dim3   `json:"num_blocks,omitempty"`
	DimBlocks      dim3   `json:"dim_blocks,omitempty"`
	SharedMemBytes int    `json:"shared_mem_bytes,omitempty"`
}

func handleLaunchKernel(w http.ResponseWriter, r *http.Request) {
	dataStr, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	dataJSON := launchKernelInput{}
	err = json.Unmarshal(dataStr, &dataJSON)
	if err != nil {
		http.Error(w, "invalid input", 400)
	}

	rawCodeObject, err := base64.StdEncoding.DecodeString(
		dataJSON.CodeObject)
	if err != nil {
		panic(err)
	}
	hsaCo := insts.NewHsaCoFromData(rawCodeObject)

	rawArgs, err := base64.StdEncoding.DecodeString(dataJSON.Args)
	if err != nil {
		panic(err)
	}

	serverInstance.driver.LaunchKernel(
		serverInstance.ctx,
		hsaCo,
		[3]uint32{
			uint32(dataJSON.NumBlocks.X * dataJSON.DimBlocks.X),
			uint32(dataJSON.NumBlocks.Y * dataJSON.DimBlocks.Y),
			uint32(dataJSON.NumBlocks.Z * dataJSON.DimBlocks.Z),
		},
		[3]uint16{
			uint16(dataJSON.DimBlocks.X),
			uint16(dataJSON.DimBlocks.Y),
			uint16(dataJSON.DimBlocks.Z),
		},
		rawArgs,
	)

	w.Write([]byte("{}"))
}
