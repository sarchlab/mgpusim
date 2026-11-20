package sample

import (
	"errors"
	"flag"
	"log"
	"os"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

var waferConfigHTMLFlag = flag.String("wafer-config-html", "",
	`Write the configuration page of a waferscale GPU to this HTML file and exit
without running the benchmark. Needs -timing -gpu waferscale.`)

// writeWaferConfigPage writes the configuration page of the wafer-scale
// platform that the simulation holds.
func writeWaferConfigPage(path string, c Config, s *simulation.Simulation) error {
	if _, ok := waferscale.ParseGPUType(c.GPUType); !c.Timing || !ok {
		return errors.New("-wafer-config-html needs -timing -gpu waferscale")
	}

	return waferscale.WriteConfigPageFile(path, c.waferMesh(), c.GPUType, s.Components())
}

// waferMesh returns the mesh of a waferscale GPU that the configuration
// selects, the default mesh when it selects none.
func (c Config) waferMesh() waferscale.Mesh {
	if c.WaferMeshWidth != 0 && c.WaferMeshHeight != 0 {
		return waferscale.Mesh{Width: c.WaferMeshWidth, Height: c.WaferMeshHeight}
	}

	return waferscale.DefaultMesh
}

// writeWaferConfigPageAndExit writes the page the -wafer-config-html flag
// asks for and ends the program without running the benchmark.
func writeWaferConfigPageAndExit(c Config, s *simulation.Simulation) {
	if err := writeWaferConfigPage(*waferConfigHTMLFlag, c, s); err != nil {
		log.Fatal(err)
	}

	if err := s.Terminate(); err != nil {
		log.Fatal(err)
	}

	log.Printf("wrote the wafer-scale GPU configuration to %s", *waferConfigHTMLFlag)
	os.Exit(0)
}
