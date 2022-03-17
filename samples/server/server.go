package main

import (
	"flag"
	"log"
	"net/http"

	"gitlab.com/akita/mgpusim/v3/samples/runner"
	"gitlab.com/akita/mgpusim/v3/server"
)

func main() {
	flag.Parse()

	runner := new(runner.Runner).ParseFlag().Init()

	server.MakeBuilder().WithDriver(runner.Driver()).Build()
	server.RegisterHandlers()
	log.Fatal(http.ListenAndServe(":8081", nil))
}
