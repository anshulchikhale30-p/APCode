package benchmark

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// cpuWork performs a single unit of CPU work.
// This is a deterministic computation that cannot be optimized away.
func cpuWork() uint64 {
	// Use a simple but non-trivial computation
	// that the compiler cannot easily optimize away.
	var sum uint64
	for i := 0; i < 1000; i++ {
		sum += uint64(i) * uint64(i+1)
		sum ^= sum >> 33
		sum *= 0xff51afd7ed558ccd
		sum ^= sum >> 33
		sum *= 0xc4ceb9fe1a85ec53
		sum ^= sum >> 33
	}
	return sum
}

// runCPUBenchmark executes the CPU benchmark with the given configuration.
func runCPUBenchmark(ctx context.Context, config Config, logicalCPUs int) CPUResult {
	// Check context before starting
	select {
	case <-ctx.Done():
		return CPUResult{
			Success: false,
			Error:   ctx.Err(),
		}
	default:
	}

	workers := config.CPUWorkers
	if workers <= 0 {
		workers = logicalCPUs
	}
	if workers <= 0 {
		workers = 1
	}
	// Cap workers to avoid excessive goroutines
	if workers > 256 {
		workers = 256
	}

	workPerWorker := config.CPUWorkUnits
	if workPerWorker <= 0 {
		workPerWorker = 100000
	}

	var totalOps int64
	var wg sync.WaitGroup
	wg.Add(workers)

	start := time.Now()

	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			var localOps int64
			for i := int64(0); i < workPerWorker; i++ {
				select {
				case <-ctx.Done():
					atomic.AddInt64(&totalOps, localOps)
					return
				default:
					cpuWork()
					localOps++
				}
			}
			atomic.AddInt64(&totalOps, localOps)
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	ops := atomic.LoadInt64(&totalOps)
	var opsPerSec float64
	if duration > 0 {
		opsPerSec = float64(ops) / duration.Seconds()
	}

	return CPUResult{
		Duration:         duration,
		Operations:       ops,
		OperationsPerSec: opsPerSec,
		Workers:          workers,
		Success:          true,
	}
}

// RunCPUBenchmark is a public function to run just the CPU benchmark.
func RunCPUBenchmark(ctx context.Context, config Config, logicalCPUs int) CPUResult {
	return runCPUBenchmark(ctx, config, logicalCPUs)
}

// ErrMemoryBenchmark is returned when memory benchmark fails.
var ErrMemoryBenchmark = errors.New("memory benchmark failed")

// runMemoryBenchmark executes the memory throughput benchmark.
func runMemoryBenchmark(ctx context.Context, config Config) MemoryResult {
	size := config.MemorySizeBytes
	if size == 0 {
		size = 64 * 1024 * 1024 // 64 MiB default
	}

	// Cap at 512 MiB to be safe for low-memory laptops
	maxSize := uint64(512 * 1024 * 1024)
	if size > maxSize {
		size = maxSize
	}

	passes := config.MemoryPasses
	if passes <= 0 {
		passes = 3
	}

	// Allocate buffer
	buf := make([]byte, size)
	if buf == nil {
		return MemoryResult{
			Success: false,
			Error:   ErrMemoryBenchmark,
		}
	}

	// Fill with pattern to ensure pages are allocated
	for i := range buf {
		buf[i] = byte(i)
	}

	var totalBytes uint64
	start := time.Now()

	for pass := 0; pass < passes; pass++ {
		select {
		case <-ctx.Done():
			duration := time.Since(start)
			return MemoryResult{
				BytesProcessed: totalBytes,
				Duration:       duration,
				BytesPerSec:    float64(totalBytes) / duration.Seconds(),
				Passes:         pass,
				Success:        false,
				Error:          ctx.Err(),
			}
		default:
			// Read and write pattern - this exercises memory bandwidth
			var sum uint64
			for i := 0; i < len(buf); i += 64 { // cache line stride
				sum += uint64(buf[i])
				buf[i] = byte(sum)
			}
			totalBytes += uint64(len(buf)) * 2 // read + write
			runtime.Gosched()                  // yield to allow cancellation
		}
	}

	duration := time.Since(start)
	var bytesPerSec float64
	if duration > 0 {
		bytesPerSec = float64(totalBytes) / duration.Seconds()
	}

	return MemoryResult{
		BytesProcessed: totalBytes,
		Duration:       duration,
		BytesPerSec:    bytesPerSec,
		Passes:         passes,
		Success:        true,
	}
}

// RunMemoryBenchmark is a public function to run just the memory benchmark.
func RunMemoryBenchmark(ctx context.Context, config Config) MemoryResult {
	return runMemoryBenchmark(ctx, config)
}
