package runner

import (
	"fmt"
	"os"
)

type metric struct {
	where string
	what  string
	value float64
}

type collector struct {
	metrics []metric
}

func (c *collector) Collect(where, what string, value float64) {
	c.metrics = append(c.metrics, metric{
		where: where,
		what:  what,
		value: value,
	})
}

func (c *collector) Dump() {
	f, err := os.Create("metrics.csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fmt.Fprintf(f, ", where, what, value\n")
	for i, m := range c.metrics {
		fmt.Fprintf(f, "%d, %s, %s, %.12f\n",
			i, m.where, m.what, m.value)
	}
}
