package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/sarchlab/mgpusim/v5/amd/samples/sample"
	"github.com/sarchlab/mgpusim/v5/amd/server"
)

func main() {
	flag.Parse()

	config := sample.ParseConfig()
	simulation, runtime, finish := config.Build()
	defer finish()

	server.MakeBuilder().WithRuntime(runtime).WithSimulation(simulation).Build()
	if err := simulation.Initialize(); err != nil {
		panic(err)
	}
	server.RegisterHandlers()
	log.Fatal(http.ListenAndServe(":8081", nil))
}
