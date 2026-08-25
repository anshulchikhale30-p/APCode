// Package benchmark defines the contract for measuring real hardware
// capability so APCode can match a model tier to the machine.
//
// No benchmarks are implemented yet; APCode deliberately refuses to
// fabricate performance numbers.
package benchmark

import (
	"context"
	"errors"
	"time"

	"apcode/internal/hardware"
)

// ErrNotImplemented is returned until real benchmarks exist.
var ErrNotImplemented = errors.New("benchmark: not implemented")

// Config holds benchmark configuration with safe defaults.
type Config struct {
	// CPUWorkers is the number of worker goroutines for CPU benchmark.
	// Default: number of logical CPUs.
	CPUWorkers int

	// CPUWorkUnits is the number of work units each CPU worker performs.
	// Default: 100000
	CPUWorkUnits int64

	// MemorySizeBytes is the amount of memory to process in memory benchmark.
	// Default: 64 MiB (conservative for low-memory laptops)
	MemorySizeBytes uint64

	// MemoryPasses is the number of times to process the memory buffer.
	// Default: 3
	MemoryPasses int

	// StorageEnabled enables the storage benchmark.
	// Default: false (disabled by default for safety)
	StorageEnabled bool

	// StorageSizeBytes is the size of the temporary file for storage benchmark.
	// Default: 256 MiB
	StorageSizeBytes uint64

	// Timeout is the maximum duration for the entire benchmark suite.
	// Default: 60 seconds
	Timeout time.Duration

	// CPUEnabled enables the CPU benchmark.
	// Default: true
	CPUEnabled bool

	// MemoryEnabled enables the memory benchmark.
	// Default: true
	MemoryEnabled bool
}

// DefaultConfig returns a Config with safe defaults for ordinary laptops.
func DefaultConfig() Config {
	return Config{
		CPUWorkers:       0, // 0 means use logical CPU count
		CPUWorkUnits:     100000,
		MemorySizeBytes:  64 * 1024 * 1024, // 64 MiB
		MemoryPasses:     3,
		StorageEnabled:   false,
		StorageSizeBytes: 256 * 1024 * 1024, // 256 MiB
		Timeout:          60 * time.Second,
		CPUEnabled:       true,
		MemoryEnabled:    true,
	}
}

// Validate checks the configuration for validity.
func (c *Config) Validate() error {
	if c.CPUWorkUnits <= 0 {
		return errors.New("benchmark: CPUWorkUnits must be positive")
	}
	if c.MemorySizeBytes == 0 {
		return errors.New("benchmark: MemorySizeBytes must be positive")
	}
	if c.MemoryPasses <= 0 {
		return errors.New("benchmark: MemoryPasses must be positive")
	}
	if c.StorageEnabled && c.StorageSizeBytes == 0 {
		return errors.New("benchmark: StorageSizeBytes must be positive when storage enabled")
	}
	if c.Timeout <= 0 {
		return errors.New("benchmark: Timeout must be positive")
	}
	return nil
}

// CPUResult holds raw CPU benchmark measurements.
type CPUResult struct {
	// Duration is the elapsed time for the CPU benchmark.
	Duration time.Duration

	// Operations is the total number of work units completed.
	Operations int64

	// OperationsPerSec is the throughput (operations/second).
	OperationsPerSec float64

	// Workers is the number of workers used.
	Workers int

	// Success indicates whether the benchmark completed successfully.
	Success bool

	// Error holds any error that occurred.
	Error error
}

// MemoryResult holds raw memory benchmark measurements.
type MemoryResult struct {
	// BytesProcessed is the total bytes read/written.
	BytesProcessed uint64

	// Duration is the elapsed time for the memory benchmark.
	Duration time.Duration

	// BytesPerSec is the throughput (bytes/second).
	BytesPerSec float64

	// Passes is the number of memory passes completed.
	Passes int

	// Success indicates whether the benchmark completed successfully.
	Success bool

	// Error holds any error that occurred.
	Error error
}

// StorageResult holds raw storage benchmark measurements.
type StorageResult struct {
	// WriteBytes is the number of bytes written.
	WriteBytes uint64

	// WriteDuration is the elapsed time for write operations.
	WriteDuration time.Duration

	// WriteBytesPerSec is the write throughput (bytes/second).
	WriteBytesPerSec float64

	// ReadBytes is the number of bytes read.
	ReadBytes uint64

	// ReadDuration is the elapsed time for read operations.
	ReadDuration time.Duration

	// ReadBytesPerSec is the read throughput (bytes/second).
	ReadBytesPerSec float64

	// Success indicates whether the benchmark completed successfully.
	Success bool

	// Error holds any error that occurred.
	Error error
}

// Result holds measured hardware capability scores.
type Result struct {
	// Profile is the hardware profile at the time of benchmarking.
	Profile hardware.HardwareProfile

	// CPU holds CPU benchmark results.
	CPU CPUResult

	// Memory holds memory benchmark results.
	Memory MemoryResult

	// Storage holds storage benchmark results.
	Storage StorageResult

	// Version is the benchmark version.
	Version string

	// Timestamp is when the benchmark was run.
	Timestamp time.Time

	// Duration is the total benchmark duration.
	Duration time.Duration
}

// Runner executes real measurements against the local machine.
type Runner interface {
	// Run measures the machine and returns a capability result.
	Run(ctx context.Context, profile hardware.HardwareProfile, config Config) (Result, error)
}

// BenchmarkRunner is the default benchmark runner implementation.
type BenchmarkRunner struct{}

// Run executes all enabled benchmarks and returns the results.
func (r *BenchmarkRunner) Run(ctx context.Context, profile hardware.HardwareProfile, config Config) (Result, error) {
	if err := config.Validate(); err != nil {
		return Result{}, err
	}

	startTime := time.Now()
	result := Result{
		Profile:   profile,
		Version:   "1.0.0",
		Timestamp: startTime,
	}

	// Run CPU benchmark
	if config.CPUEnabled {
		result.CPU = runCPUBenchmark(ctx, config, profile.LogicalCPUs)
	}

	// Run memory benchmark
	if config.MemoryEnabled {
		result.Memory = runMemoryBenchmark(ctx, config)
	}

	// Run storage benchmark
	if config.StorageEnabled {
		result.Storage = runStorageBenchmark(ctx, config)
	} else {
		result.Storage = StorageResult{
			Success: false,
			Error:   errors.New("storage benchmark disabled"),
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}
