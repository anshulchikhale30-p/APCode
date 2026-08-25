package benchmark

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

var (
	// ErrStorageBenchmark is returned when storage benchmark fails.
	ErrStorageBenchmark = errors.New("storage benchmark failed")

	// ErrStorageDisabled is returned when storage benchmark is disabled.
	ErrStorageDisabled = errors.New("storage benchmark disabled")
)

// runStorageBenchmark executes the storage throughput benchmark using a temporary file.
func runStorageBenchmark(ctx context.Context, config Config) StorageResult {
	size := config.StorageSizeBytes
	if size == 0 {
		size = 256 * 1024 * 1024 // 256 MiB default
	}

	// Cap at 1 GiB to be safe
	maxSize := uint64(1024 * 1024 * 1024)
	if size > maxSize {
		size = maxSize
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "apcode-benchmark-*")
	if err != nil {
		return StorageResult{
			Success: false,
			Error:   err,
		}
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	// Prepare test data (use a pattern that's not easily compressible)
	blockSize := 4 * 1024 // 4 KiB blocks
	numBlocks := int(size / uint64(blockSize))
	if numBlocks == 0 {
		numBlocks = 1
	}

	testBlock := make([]byte, blockSize)
	for i := range testBlock {
		testBlock[i] = byte(i ^ (i >> 8) ^ (i >> 16))
	}

	result := StorageResult{}

	// Write benchmark
	writeStart := time.Now()
	written := uint64(0)
	for i := 0; i < numBlocks; i++ {
		select {
		case <-ctx.Done():
			result.WriteDuration = time.Since(writeStart)
			result.WriteBytes = written
			if result.WriteDuration > 0 {
				result.WriteBytesPerSec = float64(written) / result.WriteDuration.Seconds()
			}
			result.Success = false
			result.Error = ctx.Err()
			return result
		default:
			n, err := tmpFile.Write(testBlock)
			if err != nil {
				result.WriteDuration = time.Since(writeStart)
				result.WriteBytes = written
				if result.WriteDuration > 0 {
					result.WriteBytesPerSec = float64(written) / result.WriteDuration.Seconds()
				}
				result.Success = false
				result.Error = err
				return result
			}
			written += uint64(n)
		}
	}

	// Ensure data is flushed to disk
	if err := tmpFile.Sync(); err != nil {
		result.WriteDuration = time.Since(writeStart)
		result.WriteBytes = written
		if result.WriteDuration > 0 {
			result.WriteBytesPerSec = float64(written) / result.WriteDuration.Seconds()
		}
		result.Success = false
		result.Error = err
		return result
	}

	result.WriteDuration = time.Since(writeStart)
	result.WriteBytes = written
	if result.WriteDuration > 0 {
		result.WriteBytesPerSec = float64(written) / result.WriteDuration.Seconds()
	}

	// Read benchmark - seek to beginning
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		result.Success = false
		result.Error = err
		return result
	}

	readBuf := make([]byte, blockSize)
	readStart := time.Now()
	readBytes := uint64(0)

	for i := 0; i < numBlocks; i++ {
		select {
		case <-ctx.Done():
			result.ReadDuration = time.Since(readStart)
			result.ReadBytes = readBytes
			if result.ReadDuration > 0 {
				result.ReadBytesPerSec = float64(readBytes) / result.ReadDuration.Seconds()
			}
			result.Success = false
			result.Error = ctx.Err()
			return result
		default:
			n, err := tmpFile.Read(readBuf)
			if err != nil && err != io.EOF {
				result.ReadDuration = time.Since(readStart)
				result.ReadBytes = readBytes
				if result.ReadDuration > 0 {
					result.ReadBytesPerSec = float64(readBytes) / result.ReadDuration.Seconds()
				}
				result.Success = false
				result.Error = err
				return result
			}
			if n == 0 {
				break
			}
			readBytes += uint64(n)
			if err == io.EOF {
				break
			}
		}
	}

	result.ReadDuration = time.Since(readStart)
	result.ReadBytes = readBytes
	if result.ReadDuration > 0 {
		result.ReadBytesPerSec = float64(readBytes) / result.ReadDuration.Seconds()
	}

	result.Success = true
	return result
}

// RunStorageBenchmark is a public function to run just the storage benchmark.
func RunStorageBenchmark(ctx context.Context, config Config) StorageResult {
	if !config.StorageEnabled {
		return StorageResult{
			Success: false,
			Error:   ErrStorageDisabled,
		}
	}
	return runStorageBenchmark(ctx, config)
}
