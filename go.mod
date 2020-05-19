module gitlab.com/akita/mgpusim

require (
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang/mock v1.4.3
	github.com/golang/protobuf v1.4.1 // indirect
	github.com/klauspost/compress v1.10.5 // indirect
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.10.0
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.3.0
	github.com/vbauerster/mpb/v4 v4.12.2
	gitlab.com/akita/akita v1.10.1
	gitlab.com/akita/mem v1.8.3
	gitlab.com/akita/noc v1.4.0
	gitlab.com/akita/util v0.5.0
	golang.org/x/crypto v0.0.0-20200429183012-4b2356b1ed79 // indirect
	golang.org/x/net v0.0.0-20200506145744-7e3656a0809f // indirect
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a // indirect
	golang.org/x/sys v0.0.0-20200509044756-6aff5f38e54f // indirect
	gopkg.in/yaml.v2 v2.2.8 // indirect
)

// replace gitlab.com/akita/akita => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

go 1.13
