package server

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

type dim3 struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

type launchKernelInput struct {
	CodeObject     string `json:"code_object,omitempty"`
	Args           string `json:"args,omitempty"`
	NumBlocks      dim3   `json:"num_blocks"`
	DimBlocks      dim3   `json:"dim_blocks"`
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

	// Load from the raw bytes (not a pre-parsed *elf.File) so the loader can
	// read the ELF e_flags and recover the GPU generation; this is required
	// for gfx8 (GCN3) Code Object V5 kernels to get separate work-item-ID
	// VGPRs instead of the gfx9+/CDNA packed-into-v0 layout.
	hsaCo := insts.LoadKernelCodeObjectFromBytes(rawCodeObject, "")

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
