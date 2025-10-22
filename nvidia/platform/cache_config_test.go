package platform_test

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
)

func TestValidateCacheConfig(t *testing.T) {
	tests := []struct {
		name               string
		config             platform.CacheConfig
		maxTransactionSize uint64
		expectError        bool
	}{
		{
			name:               "Valid 512-byte block for 512-byte transaction",
			config:             platform.DefaultL1CacheConfig(),
			maxTransactionSize: 512,
			expectError:        false,
		},
		{
			name: "Invalid 128-byte block for 512-byte transaction",
			config: platform.CacheConfig{
				Log2BlockSize:         7, // 128 bytes
				TotalByteSize:         128 * 1024,
				WayAssociativity:      8,
				NumMSHREntry:          32,
				NumBanks:              4,
				DirectoryLatency:      2,
				BankLatency:           20,
				NumReqPerCycle:        4,
				MaxNumConcurrentTrans: 64,
			},
			maxTransactionSize: 512,
			expectError:        true,
		},
		{
			name: "Valid 1024-byte block for 512-byte transaction",
			config: platform.CacheConfig{
				Log2BlockSize:         10, // 1024 bytes
				TotalByteSize:         128 * 1024,
				WayAssociativity:      8,
				NumMSHREntry:          32,
				NumBanks:              4,
				DirectoryLatency:      2,
				BankLatency:           20,
				NumReqPerCycle:        4,
				MaxNumConcurrentTrans: 64,
			},
			maxTransactionSize: 512,
			expectError:        false,
		},
		{
			name: "Valid 64-byte block for 64-byte transaction",
			config: platform.CacheConfig{
				Log2BlockSize:         6, // 64 bytes
				TotalByteSize:         128 * 1024,
				WayAssociativity:      8,
				NumMSHREntry:          32,
				NumBanks:              4,
				DirectoryLatency:      2,
				BankLatency:           20,
				NumReqPerCycle:        4,
				MaxNumConcurrentTrans: 64,
			},
			maxTransactionSize: 64,
			expectError:        false,
		},
		{
			name: "Invalid 64-byte block for 128-byte transaction",
			config: platform.CacheConfig{
				Log2BlockSize:         6, // 64 bytes
				TotalByteSize:         128 * 1024,
				WayAssociativity:      8,
				NumMSHREntry:          32,
				NumBanks:              4,
				DirectoryLatency:      2,
				BankLatency:           20,
				NumReqPerCycle:        4,
				MaxNumConcurrentTrans: 64,
			},
			maxTransactionSize: 128,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := platform.ValidateCacheConfig(tt.config, tt.maxTransactionSize)
			if tt.expectError && err == nil {
				t.Errorf("ValidateCacheConfig() expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("ValidateCacheConfig() unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultConfigs(t *testing.T) {
	// Test that default configs are valid for typical transaction sizes
	l1Config := platform.DefaultL1CacheConfig()
	if err := platform.ValidateCacheConfig(l1Config, 512); err != nil {
		t.Errorf("DefaultL1CacheConfig() is invalid for 512-byte transactions: %v", err)
	}

	l2Config := platform.DefaultL2CacheConfig()
	if err := platform.ValidateCacheConfig(l2Config, 512); err != nil {
		t.Errorf("DefaultL2CacheConfig() is invalid for 512-byte transactions: %v", err)
	}
}

func TestCacheBlockSizes(t *testing.T) {
	tests := []struct {
		log2BlockSize uint64
		expectedSize  uint64
	}{
		{6, 64},
		{7, 128},
		{8, 256},
		{9, 512},
		{10, 1024},
	}

	for _, tt := range tests {
		blockSize := uint64(1) << tt.log2BlockSize
		if blockSize != tt.expectedSize {
			t.Errorf("log2BlockSize %d should give %d bytes, got %d bytes",
				tt.log2BlockSize, tt.expectedSize, blockSize)
		}
	}
}
