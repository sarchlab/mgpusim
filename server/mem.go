package server

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"gitlab.com/akita/mgpusim/v2/driver"
)

type mallocInput struct {
	Size uint64
}

type mallocOutput struct {
	Ptr uint64 `json:"ptr"`
}

func handleMalloc(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	dataStr := query.Get("data")
	dataJSON := mallocInput{}

	err := json.Unmarshal([]byte(dataStr), &dataJSON)
	if err != nil {
		http.Error(w, "invalid input", 400)
	}

	ptr := serverInstance.driver.AllocateMemory(
		serverInstance.ctx, dataJSON.Size)

	output := mallocOutput{Ptr: uint64(ptr)}

	rspData, err := json.Marshal(output)
	if err != nil {
		panic(err)
	}

	w.Write(rspData)
}

func handleFree(w http.ResponseWriter, r *http.Request) {
	ptrStr := mux.Vars(r)["ptr"]
	ptr, err := strconv.ParseUint(ptrStr, 10, 64)
	if err != nil {
		http.Error(w, "ptr is not valid", 400)
	}

	err = serverInstance.driver.FreeMemory(
		serverInstance.ctx, driver.GPUPtr(ptr))
	if err != nil {
		http.Error(w, "failed to free the memory", 400)
	}

	w.Write([]byte("{}"))
}

type memcopyH2DInput struct {
	Ptr  uint64
	Data string
}

func handleMemcopyH2D(w http.ResponseWriter, r *http.Request) {
	dataStr, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	input := memcopyH2DInput{}
	err = json.Unmarshal(dataStr, &input)
	if err != nil {
		http.Error(w, "invalid input", 400)
	}

	rawData, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		http.Error(w, "invalid input", 400)
	}

	serverInstance.driver.MemCopyH2D(serverInstance.ctx,
		driver.GPUPtr(input.Ptr), rawData)

	output := "{}"
	w.Write([]byte(output))
}

type memcopyD2HInput struct {
	Ptr  uint64
	Size uint64
}

type memcopyD2HOutput struct {
	Data string `json:"data"`
}

func handleMemcopyD2H(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	dataStr := query.Get("data")
	dataJSON := memcopyD2HInput{}

	err := json.Unmarshal([]byte(dataStr), &dataJSON)
	if err != nil {
		http.Error(w, "invalid input", 400)
	}

	rawData := make([]byte, dataJSON.Size)

	serverInstance.driver.MemCopyD2H(serverInstance.ctx,
		rawData, driver.GPUPtr(dataJSON.Ptr))

	encodedData := base64.StdEncoding.EncodeToString(rawData)

	output := memcopyD2HOutput{Data: encodedData}
	rspData, err := json.Marshal(output)
	if err != nil {
		panic(err)
	}

	w.Write(rspData)
}
