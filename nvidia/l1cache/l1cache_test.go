package l1cache_test

import (
	"testing"

	cache "github.com/sarchlab/mgpusim/v4/nvidia/l1cache"
)

func TestL1CacheBuild(t *testing.T) {
	c := cache.NewL1Cache("TestCache", nil, 1000, 6, 4, 32*1024, 1)
	if c.Name != "TestCache" {
		t.Errorf("Expected cache name to be TestCache, got %s", c.Name)
	}
}
