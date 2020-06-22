// Package layers provides implementations of DNN layers that run on MGPUSim.
package layers

//go:generate esc -private -pkg=$GOPACKAGE -o=bindata.go trans.hsaco gpu_gemm.hsaco relu.hsaco
