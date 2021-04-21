module gitlab.com/akita/mgpusim/v2

require (
	github.com/aws/aws-sdk-go v1.37.33 // indirect
	github.com/fatih/color v1.10.0
	github.com/golang/mock v1.5.0
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo v1.15.2
	github.com/onsi/gomega v1.11.0
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.3.0
	gitlab.com/akita/akita/v2 v2.0.1
	gitlab.com/akita/dnn v0.5.2
	gitlab.com/akita/mem/v2 v2.0.0
	gitlab.com/akita/noc/v2 v2.0.0
	gitlab.com/akita/util/v2 v2.0.0
	golang.org/x/crypto v0.0.0-20210317152858-513c2a44f670 // indirect
	golang.org/x/sys v0.0.0-20210317225723-c4fcb01b228e // indirect
	gonum.org/v1/gonum v0.9.0 // indirect
)

// replace github.com/syifan/goseth => ../goseth

// replace gitlab.com/akita/akita/v2 => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem/v2 => ../mem

// replace gitlab.com/akita/util => ../util

// replace gitlab.com/akita/dnn => ../dnn

go 1.16
