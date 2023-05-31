package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/sarchlab/mgpusim/v3/samples/runner"
	"github.com/sarchlab/mgpusim/v3/server"
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	server.MakeBuilder().WithDriver(runner.Driver()).Build()
	server.RegisterHandlers()
	log.Fatal(http.ListenAndServe(":8081", nil))
}
