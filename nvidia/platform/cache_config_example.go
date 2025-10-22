// Package platform provides example cache configuration for NVIDIA GPU simulation
// 
// This example demonstrates how to configure caches with appropriate block sizes
// to avoid "slice bounds out of range" errors when processing large memory transactions.
//
// IMPORTANT: This is a template. Adapt it to your specific nvidia_cache branch implementation.

package platform

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writearound"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	log "github.com/sirupsen/logrus"
)

// CacheConfig holds cache configuration parameters
type CacheConfig struct {
	// Log2BlockSize is the log2 of the cache block size in bytes
	// For 512-byte blocks, use 9 (2^9 = 512)
	// For 1024-byte blocks, use 10 (2^10 = 1024)
	Log2BlockSize uint64
	
	// TotalByteSize is the total size of the cache in bytes
	TotalByteSize uint64
	
	// WayAssociativity is the number of ways in the cache
	WayAssociativity int
	
	// NumMSHREntry is the number of Miss Status Holding Register entries
	NumMSHREntry int
	
	// NumBanks is the number of banks in the cache for parallel access
	NumBanks int
	
	// DirectoryLatency is the number of cycles to access the directory
	DirectoryLatency int
	
	// BankLatency is the number of cycles to access a bank
	BankLatency int
	
	// NumReqPerCycle is the number of requests processed per cycle
	NumReqPerCycle int
	
	// MaxNumConcurrentTrans is the maximum number of concurrent transactions
	MaxNumConcurrentTrans int
}

// DefaultL1CacheConfig returns a configuration suitable for L1 cache with 512-byte blocks
func DefaultL1CacheConfig() CacheConfig {
	return CacheConfig{
		Log2BlockSize:         9,    // 512 bytes - handles large memory transactions
		TotalByteSize:         128 * mem.KB,
		WayAssociativity:      8,
		NumMSHREntry:          32,
		NumBanks:              4,
		DirectoryLatency:      2,
		BankLatency:           20,
		NumReqPerCycle:        4,
		MaxNumConcurrentTrans: 64,
	}
}

// DefaultL2CacheConfig returns a configuration suitable for L2 cache
func DefaultL2CacheConfig() CacheConfig {
	return CacheConfig{
		Log2BlockSize:         9,    // 512 bytes - same as L1 for consistency
		TotalByteSize:         4 * mem.MB,
		WayAssociativity:      16,
		NumMSHREntry:          64,
		NumBanks:              16,
		DirectoryLatency:      4,
		BankLatency:           40,
		NumReqPerCycle:        8,
		MaxNumConcurrentTrans: 128,
	}
}

// BuildCache creates a writearound cache with the given configuration
func BuildCache(
	engine sim.Engine,
	freq sim.Freq,
	config CacheConfig,
	name string,
	remotePorts ...sim.RemotePort,
) *writearound.Comp {
	// Validate block size
	blockSize := uint64(1) << config.Log2BlockSize
	log.WithFields(log.Fields{
		"name":       name,
		"blockSize":  blockSize,
		"totalSize":  config.TotalByteSize,
		"ways":       config.WayAssociativity,
	}).Info("Building cache")

	builder := writearound.MakeBuilder().
		WithEngine(engine).
		WithFreq(freq).
		WithLog2BlockSize(config.Log2BlockSize).
		WithTotalByteSize(config.TotalByteSize).
		WithWayAssociativity(config.WayAssociativity).
		WithNumMSHREntry(config.NumMSHREntry).
		WithNumBanks(config.NumBanks).
		WithDirectoryLatency(config.DirectoryLatency).
		WithBankLatency(config.BankLatency).
		WithNumReqsPerCycle(config.NumReqPerCycle).
		WithMaxNumConcurrentTrans(config.MaxNumConcurrentTrans)

	if len(remotePorts) > 0 {
		builder = builder.WithRemotePorts(remotePorts...)
		if len(remotePorts) == 1 {
			builder = builder.WithAddressMapperType("single")
		} else {
			builder = builder.WithAddressMapperType("interleaved")
		}
	}

	return builder.Build(name)
}

// ValidateCacheConfig checks if the cache configuration is valid
func ValidateCacheConfig(config CacheConfig, maxTransactionSize uint64) error {
	blockSize := uint64(1) << config.Log2BlockSize
	
	if blockSize < maxTransactionSize {
		return fmt.Errorf(
			"cache block size (%d bytes) is smaller than max transaction size (%d bytes): "+
				"increase Log2BlockSize to at least %d",
			blockSize,
			maxTransactionSize,
			getLog2(maxTransactionSize),
		)
	}
	
	if config.TotalByteSize < blockSize*uint64(config.WayAssociativity) {
		return fmt.Errorf(
			"cache size (%d bytes) is too small for block size (%d bytes) and associativity (%d)",
			config.TotalByteSize,
			blockSize,
			config.WayAssociativity,
		)
	}
	
	return nil
}

// getLog2 returns the ceiling of log2(n)
func getLog2(n uint64) uint64 {
	if n == 0 {
		return 0
	}
	
	log2 := uint64(0)
	n--
	for n > 0 {
		log2++
		n >>= 1
	}
	return log2
}

// Example usage:
//
// // Create L1 cache for each SM
// l1Config := DefaultL1CacheConfig()
// if err := ValidateCacheConfig(l1Config, 512); err != nil {
//     panic(err)
// }
// l1Cache := BuildCache(engine, freq, l1Config, "SM0.L1Cache", l2Port)
//
// // Create shared L2 cache
// l2Config := DefaultL2CacheConfig()
// l2Cache := BuildCache(engine, freq, l2Config, "L2Cache", memoryPort)
