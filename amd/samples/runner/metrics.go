package runner

import (
	"fmt"
	"os"
)

type metric struct {
	where      string
	what       string
	value      float64
	header     string
	metricType string
}

type collector struct {
	metrics []metric
}

func (c *collector) Collect(where, what string, value float64) {
	c.metrics = append(c.metrics, metric{
		where:      where,
		what:       what,
		value:      value,
		metricType: "data",
	})
}

func (c *collector) CollectHeader(header string) {
	c.metrics = append(c.metrics, metric{
		header:     header,
		metricType: "header",
	})
}

func (c *collector) Dump(name string) {
	f, err := os.Create(name + ".csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fmt.Fprintf(f, ", where, what, value\n")
	for i, m := range c.metrics {
		if m.metricType == "data" {
			fmt.Fprintf(f, "%d, %s, %s, %.12f\n",
				i, m.where, m.what, m.value)
		} else if m.metricType == "header" {
			fmt.Fprintf(f, "%d, -, %s, -\n", i, m.header)
		}
	}
}
