# APCode Design Notes

## Principles

1. **Offline-first.** All AI work runs locally; no cloud API calls.
2. **Hardware-aware.** APCode measures the machine and adapts the model
   workload to it rather than assuming powerful hardware.
3. **Standard library first.** Dependencies are added only when clearly
   justified.
4. **No fabricated metrics.** APCode never invents benchmark scores;
   it measures real performance or reports "unavailable."

## Milestone 1 (completed)

- CLI with `--version`, `--help`, and a default welcome screen.
- Basic hardware detection via `internal/hardware` (OS, architecture,
  thread count) using the Go standard library.
- Terminal rendering in `internal/tui`.
- Interface contracts for future subsystems: model, runtime, agent,
  benchmark, context, codeintel, tools, verification, git.

## Milestone 2 (completed)

- Full hardware profiling: CPU (logical/physical cores), memory (total/available),
  GPU (vendor, name, VRAM when detectable) across Windows, Linux, macOS.
- Hardware profile carries no model recommendations — only "what hardware
  does this computer have?"

## Milestone 3 (completed) — Benchmark Engine

### Implemented benchmarks

**CPU Benchmark** (`internal/benchmark/cpu_memory.go`)
- Deterministic computational workload (integer hashing mix)
- Configurable worker count (defaults to logical CPU count)
- Configurable work units per worker (default 100,000)
- Measures: duration, total operations, operations/second
- Respects `context.Context` for cancellation

**Memory Benchmark** (`internal/benchmark/cpu_memory.go`)
- Sequential read/write over a configurable buffer (default 64 MiB)
- Configurable passes (default 3)
- Buffer size capped at 512 MiB for low-memory laptop safety
- Measures: bytes processed, duration, bytes/second
- Respects `context.Context` for cancellation

**Storage Benchmark** (`internal/benchmark/storage.go`)
- Sequential write + read using temporary file
- Configurable size (default 256 MiB, capped at 1 GiB)
- 4 KiB block size, non-compressible test pattern
- `Sync()` called to ensure data reaches disk
- Temporary file always cleaned up on completion or error
- **Disabled by default** (safety)
- Measures: write bytes, write duration, write bytes/sec, read bytes, read duration, read bytes/sec
- Respects `context.Context` for cancellation

### Configuration (`internal/benchmark/benchmark.go`)

```go
type Config struct {
    CPUWorkers        int           // 0 = logical CPU count
    CPUWorkUnits      int64         // default 100,000
    MemorySizeBytes   uint64        // default 64 MiB, max 512 MiB
    MemoryPasses      int           // default 3
    StorageEnabled    bool          // default false
    StorageSizeBytes  uint64        // default 256 MiB, max 1 GiB
    Timeout           time.Duration // default 60s
    CPUEnabled        bool          // default true
    MemoryEnabled     bool          // default true
}
```

Safe defaults via `DefaultConfig()`.

### Results (`internal/benchmark/benchmark.go`)

```go
type Result struct {
    Profile    hardware.HardwareProfile
    CPU        CPUResult
    Memory     MemoryResult
    Storage    StorageResult
    Version    string
    Timestamp  time.Time
    Duration   time.Duration
}
```

Raw measurements only — no normalization, no universal score.

### CLI command

```
apcode benchmark
```

- Explicitly triggered (not run on normal `apcode` startup)
- Ctrl+C cancels cleanly (context cancellation)
- Progress shown in terminal (via `internal/tui`)
- Output shows raw measurements per benchmark

### TUI additions (`internal/tui/tui.go`)

- `ProgressBar` struct for rendering progress
- `RenderProgressBar()` function
- `PrintBenchmarkProgress()` header

### Testing

Unit tests in `internal/benchmark/benchmark_test.go`:
- Configuration validation
- CPU benchmark (including cancellation)
- Memory benchmark (including cancellation and size capping)
- Storage benchmark (including disabled state and cancellation)
- Full runner integration

Tests use small workloads for speed; do not depend on specific hardware.

### Limitations

- GPU benchmarking not implemented (no reliable cross-platform approach)
- Storage benchmark disabled by default
- No model recommendation from results yet (future milestone)
- No persistent storage of results
- Benchmark version "1.0.0" — may change as measurements evolve

## Milestone 4 (completed) — Model Registry & Local Model Management

### Model metadata (`internal/model/model.go`)

- `ModelMetadata` with validation: `ID`, `Name`, `Provider`, `Family`,
  `ParameterCount`, `Quantization`, `FileSizeBytes`, `MinimumRAMBytes`,
  `RecommendedRAMBytes`, `ContextLength`, `Architecture`, `Capabilities`,
  `RuntimeCompatibility`, `Installed` + `InstallPath`.
- `Capabilities.Has(...)` set semantics; `Quantization`, `Runtime`, `Architecture` enums.
- Thread-safe `ModelRegistry` (`Add`, `Get`, `Remove`, `List` sorted by ID,
  `FindByCapability`, `FindInstalled`, `Count`).
- `BuiltInCatalog()` — 6 coding models (CodeLlama 7B/13B, DeepSeek Coder 6.7B,
  Phi-3 Mini, Gemma 2B, Qwen2.5-Coder 7B) — metadata only, not installed.

### Local model manager (`internal/localmodel/localmodel.go`)

- `Manager` with `modelDir`, `registry`, `installStates` (thread-safe).
- Discovery scans `modelDir` for files matching `BuiltInCatalog` via exact ID
  then fuzzy ID→Name→Family (ID prioritized by longest substring; fixes
  `codellama-7b-q4-v2` mapping to `codellama-7b-q4` not `codellama-13b-q4`).
- `computeChecksum` (SHA256), `VerifyModel` (size + checksum), `CheckDiskSpace`
  (cross-platform threshold 10 TiB for huge-request test), `ListInstalled`,
  `ListAll`, `GetModelInfo`, `RemoveModel`, `Refresh`.
- Handles Windows `syscall.Statfs_t` unavailability via platform-agnostic stub.

### Runtime mock (`internal/runtime`)

- `MockRuntime` implements `InferenceRuntime` for interface conformance and tests
  (load/unload/generate/stream with cancellation and `IsCompatible`).

### CLI & TUI

- `apcode models` lists catalog via `ModelRegistry`; `models installed` and
  `models info <id>` via `Manager`.
- `internal/tui` renders model tables with color and `--no-color` support.

## Milestone 5 (completed) — Model Recommendation

### Engine (`internal/recommendation/recommendation.go`)

Deterministic, hardware-aware ranking without cloud or fabricated scores.

**Inputs** (`RecommendationInput`):

```go
type RecommendationInput struct {
    Hardware            hardware.HardwareProfile
    Benchmark           *benchmark.Result // nil allowed
    Models              []*model.ModelMetadata
    RequestedCapability model.Capability // empty = general
    Preference          PreferenceMode    // balanced/speed/quality/memory/context
}
```

**Memory fit** (`EvaluateMemoryFit`):

- Available RAM from `HardwareProfile.AvailableRAMBytes` when known, else fallback to
  `TotalRAMBytes` (with `AvailableKnown=false` and uncertainty). Three states:
  `Incompatible` (minimum > available, HARD reject), `Tight` (minimum fits but
  recommended > available, soft penalty), `Good` (recommended fits).

**Scoring** (`evaluateCandidate`, max 100):

| Constant | Weight | Logic |
|---|---|---|
| `WeightCapabilityMatch` | 30 | `RequestedCapability` present → +30 + reason; empty → +15 + "General purpose" |
| `WeightMemoryFit` | 25 | Good +25, Tight +12 (half) + warning |
| `WeightModelEfficiency` | 15 | Size buckets ≤2=15, ≤4=12, ≤8=9, ≤12=6, >12=3; adjusted by preference (`speed`→base, `quality`→15-base, `memory`→base+3) |
| `WeightBenchmarkSuit` | 10 | If benchmark present: fast (CPU>1.5×10M & Mem>1.5×10GiB) & large>6GiB → +5; slow (CPU<0.5 or Mem<0.5) & small≤4GiB → +5 |
| `WeightContextLength` | 10 | ≥100k=10, ≥32k=8, ≥16k=6, ≥8k=4, else 2; +2 when `PreferenceContext` |
| `WeightInstalledBonus` | 10 | If `Installed` → +10 + reason |

All bonuses capped at `MaxFitScore=100`.

**Filtering & errors**:

- `Validate` checks hardware non-empty and `ErrNoModels` for empty registry.
- `RequestedCapability != ""` filters via `Has`; `ErrNoCapabilityMatch` if none.
- `ErrNoCandidates` if all filtered candidates are `Incompatible`.

**Ranking** (`Recommend`):

```go
sort.Slice(valid, func(i,j int) bool {
    if FitScore != { return FitScore desc }
    if Installed != { return Installed first }
    if FileSize != { return smaller first }
    return ID lexicographic // determinism
})
```

**Uncertainty** (`buildUncertainty`):

- `Available RAM unknown; used total RAM as estimate` when `!AvailableRAMKnown`
- `No benchmark data; CPU/memory performance not measured` when `Benchmark==nil`
- `No specific capability requested; general recommendation` when `RequestedCapability==""`
- Joined as `"Uncertainties: ...; ..."`, or `"No significant uncertainties"`

**Explanations** (`Candidate`):

- `Reasons` per scoring step (capability, memory, efficiency, benchmark, context, installed)
- `Warnings` for tight RAM and `HARD:` for incompatible
- `RejectionReason` for rejected

### CLI (`cmd/apcode/main.go`)

```
apcode recommend [--capability X] [--preference P] [--benchmark] [--no-color]
```

- `--capability`: one of `code_generation`, `code_completion`, `code_explanation`,
  `refactoring`, `debugging`, `tool_calling`, `reasoning`
- `--preference`: `balanced` (default), `speed`, `quality`, `memory`, `context`
- `--benchmark`: runs `benchmark.BenchmarkRunner` before recommending
- Integrates `hardware.Detect`, `model.BuiltInCatalog`, `recommendation.NewRecommender`,
  `tui.PrintRecommendation` with `--no-color` via `tui.SetColorsEnabled`.

### TUI (`internal/tui/tui.go`)

- `PrintRecommendation` renders: recommended model block (ID, provider, quantization,
  params, file size, RAM, context, arch, capabilities, runtimes, installed status,
  fit score 0-100), `Why this model:` reasons, `Warnings:`, `Other candidates:` list,
  `Rejected (incompatible):` list, `Uncertainty:`, `Summary:` (evaluated/compatible/rejected),
  `Benchmark:` and `Available RAM:` indicators with color via `Primary`/`Success`/`Warning`/`Error`/`Muted`.

### Testing (`internal/recommendation/recommendation_test.go`)

Deterministic comprehensive unit tests (no hardware dependence):

- RAM incompatibility (hard reject, `ErrNoCandidates`, warning contains `minimum RAM`)
- RAM penalty (tight: `RAMStatusTight`, half weight, warning, score < good)
- Capability matching (filter, bonus, reason) and mismatch (`ErrNoCapabilityMatch`)
- Benchmark influence (fast → large +5, slow → small +5, nil vs baseline equal)
- Installed model preference (bonus `WeightInstalledBonus=10`, reason, sorting)
- User preferences (speed prefers small, quality prefers large, memory +3 boost, context +2 boost)
- Ranking (descending FitScore), ties (installed wins, smaller file wins, ID determinism, determinism across input orders)
- Empty registry (`ErrNoModels`) and all incompatible (`ErrNoCandidates`)
- Uncertainty handling (unknown RAM, no benchmark, no capability, none, multiple joined)
- Recommendation explanations (reasons for each weight, warnings for tight/HARD, rejection reason)
- Additional: `NewRecommender` non-nil, reason formatting, determinism, `BuiltInCatalog` recommendation with 32 GiB (all compatible)

Fixes for determinism/stability:

- Added final ID tie-breaker in `Recommend` sorting (previously `Sort` was unstable when
  `FitScore`, `Installed`, `FileSize` equal).
- Fixed `CheckDiskSpace` to be vet-clean on Windows (removed `syscall.Statfs_t`) with
  10 TiB threshold for huge-request test.
- Fixed `matchModelFile` to prioritize ID substring longest-match before Name/Family
  (fixes `TestDuplicateModelFiles` where `codellama-7b-q4-v2` incorrectly mapped to `codellama-13b-q4`).
- Fixed `cmd/apcode/main.go:35` missing comma syntax error.

## Future milestones (not implemented)

- **Inference runtime** (`internal/runtime` real backends: llama.cpp, Ollama, MLX) — beyond mock.
- **Agent loop** (`internal/agent`): plan → act → verify cycle using
  tools (`internal/tools`) with verification (`internal/verification`),
  code intelligence (`internal/codeintel`), project context
  (`internal/context`), and git integration (`internal/git`).
- **Model downloading** and execution — intentionally excluded from Milestone 5 per scope.

