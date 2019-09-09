module gitlab.com/akita/gcn3

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.7.0 // indirect
	github.com/golang/mock v1.3.1
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.1.0
	gitlab.com/akita/akita v1.4.0
	gitlab.com/akita/mem v1.3.1
	gitlab.com/akita/noc v1.2.0
	gitlab.com/akita/util v0.1.8
	gitlab.com/akita/vis v0.2.0
	gopkg.in/cheggaaa/pb.v1 v1.0.28
)

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

go 1.13
