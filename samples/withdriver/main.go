package main

import "gitlab.com/yaotsu/gcn3/driver"

func main() {
	driver := driver.NewDriver("driver")

	driver.Listen()
}
