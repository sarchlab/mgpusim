module gitlab.com/akita/gcn3

require (
	github.com/golang/mock v1.3.1
	github.com/onsi/ginkgo v1.10.3
	github.com/onsi/gomega v1.5.0
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.1.0
	github.com/vbauerster/mpb/v4 v4.11.1
	gitlab.com/akita/akita v1.10.0
	gitlab.com/akita/mem v1.7.3
	gitlab.com/akita/noc v1.3.3
	gitlab.com/akita/util v0.2.2
	go.mongodb.org/mongo-driver v1.1.3 // indirect
	golang.org/x/crypto v0.0.0-20191122220453-ac88ee75c92c // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20191120155948-bd437916bb0e // indirect
	golang.org/x/text v0.3.2 // indirect
)

// replace gitlab.com/akita/akita => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

go 1.13
