package runner

import (
	"github.com/sarchlab/akita/v4/datarecording"
)

type metric struct {
	Location   string
	What       string
	Value      float64
	Header     string
	MetricType string
}

type collector struct {
	metrics  []metric
	recorder datarecording.DataRecorder
}

func (c *collector) Collect(where, what string, value float64) {
	c.metrics = append(c.metrics, metric{
		Location:   where,
		What:       what,
		Value:      value,
		MetricType: "data",
	})
}

func (c *collector) CollectHeader(header string) {
	c.metrics = append(c.metrics, metric{
		Header:     header,
		MetricType: "header",
	})
}

func (c *collector) Dump(name string) {
	c.recorder.CreateTable(name, metric{})

	for _, m := range c.metrics {
		c.recorder.InsertData(name, m)
	}
}

func newCollector(
	recorder datarecording.DataRecorder,
) *collector {
	collector := &collector{}
	collector.recorder = recorder

	return collector
}
