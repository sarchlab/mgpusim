package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

const usageMessage = "" +
	`
Usage of vis 
	vis [flags] trace.out

Flags
	-http=addr: HTTP service address (e.g., ':6060')
	`

var (
	httpFlag  = flag.String("http", "localhost:0", "HTTP service address (e.g., ':6060')")
	traceFile string
)

func main() {
	parseArgs()
	startServer()
}

func parseArgs() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usageMessage)
		os.Exit(2)
	}

	flag.Parse()

	switch flag.NArg() {
	case 1:
		traceFile = flag.Arg(0)
	default:
		flag.Usage()
	}

}

func startServer() {
	ln, err := net.Listen("tcp", *httpFlag)
	if err != nil {
		log.Panic(err)
	}

	openbrowser("http://" + ln.Addr().String())

	http.HandleFunc("/", httpMain)
	err = http.Serve(ln, nil)
	if err != nil {
		log.Panic(err)
	}
}

func httpMain(w http.ResponseWriter, r *http.Request) {
	log.Println("handling")
}

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}

}
