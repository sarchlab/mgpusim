// Package daisen2 provides a trace visualization server for Akita simulations.
package daisen2

import "github.com/sarchlab/akita/v5/daisen2/internal/httpapi"

type ProgressBar = httpapi.ProgressBar
type Server = httpapi.Server

func NewReplayServer(sqliteFile, addr string) *Server {
	return httpapi.NewReplayServer(sqliteFile, addr)
}

func NewReplayServerReadOnly(sqliteFile string) *Server {
	return httpapi.NewReplayServerReadOnly(sqliteFile)
}
