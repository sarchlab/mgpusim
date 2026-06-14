# About

[![Build Status](https://travis-ci.org/tebeka/atexit.svg?branch=master)](https://travis-ci.org/tebeka/atexit)

Simple `atexit` implementation for [Go](https://golang.org).

Note that you *have* to call `atexit.Exit` and not `os.Exit` to terminate your
program (that is, if you want the `atexit` handlers to execute).

# Example usage

```go
package main

import (
    "fmt"

    "github.com/tebeka/atexit"
)

func handler() {
    fmt.Println("Exiting")
}

func main() {
	atexit.Register(handler)
	atexit.Exit(0)
}
```

# Install

    go get github.com/tebeka/atexit

Contact
=======

* [Home](https://github.com/tebeka/atexit)
* [Author](mailto:miki.tebeka@gmail.com)
