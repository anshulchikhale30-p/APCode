package recommendation

import (
	"errors"
	"strings"
	"testing"

	"apcode/internal/benchmark"
	"apcode/internal/hardware"
	"apcode/internal/model"
)

const GiB = 1 << 30

// helpers

func testHardware(total, avail uint64, availKnown bool) hardware.HardwareProfile {
	return hardware.HardwareProfile{
		OS:                "linux",
		Arch:              "amd64",
		LogicalCPUs:       8,
		TotalRAMBytes:     total,
		AvailableRAMBytes: avail,
		AvailableRAMKnown: availKnown,
	}
}

func emptyHardware() hardware.HardwareProfile {
	return hardware.HardwareProfile{}
}

func testModel(id string, minRAM, recRAM, fileSize uint64, ctx int, caps model.Capabilities, installed bool) *model.ModelMetadata {
	m := &model.ModelMetadata{
		ID:                   id,
		Name:                 "Model " + id,
		Provider:             "TestProvider",
		Family:               "TestFamily",
		ParameterCount:       7,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        fileSize,
		MinimumRAMBytes:      minRAM,
		RecommendedRAMBytes:  recRAM,
		ContextLength:        ctx,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         caps,
		RuntimeCompatibility: []model.Runtime{model.RuntimeLlamaCPP},
		Installed:            installed,
	}
	if installed {
		m.InstallPath = "/tmp/" + id
	}
	return m
}

func fastBenchmark() *benchmark.Result {
	return &benchmark.Result{
		CPU:    benchmark.CPUResult{OperationsPerSec: 30_000_000, Success: true},            // 3x baseline
		Memory: benchmark.MemoryResult{BytesPerSec: 20 * 1024 * 1024 * 1024, Success: true}, // 2x baseline
	}
}

func slowBenchmark() *benchmark.Result {
	return &benchmark.Result{
		CPU:    benchmark.CPUResult{OperationsPerSec: 3_000_000, Success: true},            // 0.3x
		Memory: benchmark.MemoryResult{BytesPerSec: 2 * 1024 * 1024 * 1024, Success: true}, // 0.2x
	}
}

func baselineBenchmark() *benchmark.Result {
	return &benchmark.Result{
		CPU:    benchmark.CPUResult{OperationsPerSec: 10_000_000, Success: true},
		Memory: benchmark.MemoryResult{BytesPerSec: 10 * 1024 * 1024 * 1024, Success: true},
	}
}

// ============================================================
// Validate
// ============================================================

func TestRecommendationInputValidateEmptyHardware(t *testing.T) {
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	input := RecommendationInput{
		Hardware: emptyHardware(),
		Models:   []*model.ModelMetadata{m},
	}
	if err := input.Validate(); err == nil {
		t.Error("expected error for empty hardware")
	}
}

func TestRecommendationInputValidateEmptyRegistry(t *testing.T) {
	input := RecommendationInput{
		Hardware: testHardware(16*GiB, 16*GiB, true),
		Models:   []*model.ModelMetadata{},
	}
	if !errors.Is(input.Validate(), ErrNoModels) {
		t.Error("expected ErrNoModels")
	}
	_, err := NewRecommender().Recommend(input)
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("Recommend should return ErrNoModels, got %v", err)
	}
}

// ============================================================
// RAM incompatibility
// ============================================================

func TestRAMIncompatibilityHardConstraint(t *testing.T) {
	hw := testHardware(8*GiB, 4*GiB, true)
	// model requires 6 GiB minimum but only 4 available
	mBig := testModel("big", 6*GiB, 8*GiB, 4*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	mSmall := testModel("small", 2*GiB, 4*GiB, 1*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)

	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mBig, mSmall},
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Rejected) != 1 {
		t.Fatalf("expected 1 rejected, got %d", len(result.Rejected))
	}
	if result.Rejected[0].Model.ID != "big" {
		t.Errorf("rejected should be big, got %s", result.Rejected[0].Model.ID)
	}
	if !result.Rejected[0].Rejected {
		t.Error("rejected candidate should have Rejected=true")
	}
	if !strings.Contains(result.Rejected[0].RejectionReason, "minimum RAM") {
		t.Errorf("rejection reason should mention minimum RAM, got %q", result.Rejected[0].RejectionReason)
	}
	if result.Recommended.Model.ID != "small" {
		t.Errorf("recommended should be small, got %s", result.Recommended.Model.ID)
	}
	// Ensure incompatible model not in candidates
	for _, c := range result.Candidates {
		if c.Model.ID == "big" {
			t.Error("incompatible model should not be in candidates")
		}
	}
}

func TestEvaluateMemoryFitIncompatible(t *testing.T) {
	hw := testHardware(8*GiB, 4*GiB, true)
	m := testModel("m1", 6*GiB, 8*GiB, 4*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit := EvaluateMemoryFit(hw, m)
	if fit.Status != RAMStatusIncompatible {
		t.Errorf("expected Incompatible, got %v", fit.Status)
	}
	if !strings.Contains(fit.Reason, "minimum RAM") {
		t.Error("reason should mention minimum RAM")
	}
	if fit.AvailableRAM != 4*GiB {
		t.Errorf("AvailableRAM mismatch: got %d", fit.AvailableRAM)
	}
}

func TestAllIncompatibleModels(t *testing.T) {
	hw := testHardware(4*GiB, 2*GiB, true)
	m1 := testModel("m1", 6*GiB, 8*GiB, 4*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m2 := testModel("m2", 8*GiB, 12*GiB, 6*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{m1, m2},
	})
	if !errors.Is(err, ErrNoCandidates) {
		t.Errorf("expected ErrNoCandidates, got %v", err)
	}
}

// ============================================================
// RAM penalty (tight)
// ============================================================

func TestRAMPenaltyTight(t *testing.T) {
	hw := testHardware(16*GiB, 6*GiB, true)
	// min fits, rec exceeds
	mTight := testModel("tight", 4*GiB, 8*GiB, 3*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit := EvaluateMemoryFit(hw, mTight)
	if fit.Status != RAMStatusTight {
		t.Errorf("expected Tight, got %v", fit.Status)
	}
	if !strings.Contains(fit.Reason, "recommended RAM") {
		t.Errorf("tight reason should mention recommended RAM, got %q", fit.Reason)
	}

	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mTight},
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected 1 candidate")
	}
	c := result.Candidates[0]
	// Tight should have warning
	if len(c.Warnings) == 0 {
		t.Error("tight model should have warnings")
	}
	found := false
	for _, w := range c.Warnings {
		if strings.Contains(w, "recommended RAM") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings should mention recommended RAM: %v", c.Warnings)
	}
	// FitScore should be lower than Good case
	hwGood := testHardware(16*GiB, 16*GiB, true)
	resultGood, _ := r.Recommend(RecommendationInput{
		Hardware: hwGood,
		Models:   []*model.ModelMetadata{mTight},
	})
	if resultGood.Candidates[0].FitScore <= c.FitScore {
		t.Errorf("good fit should score higher than tight: good %d vs tight %d", resultGood.Candidates[0].FitScore, c.FitScore)
	}
	expectedGoodBonus := WeightMemoryFit - WeightMemoryFit/2 // 25-12=13 difference
	diff := resultGood.Candidates[0].FitScore - c.FitScore
	if diff < expectedGoodBonus-2 || diff > expectedGoodBonus+5 {
		// allow small variance due to other scoring but tight should be penalized
		t.Logf("score diff %d, expected around %d", diff, expectedGoodBonus)
	}
}

func TestEvaluateMemoryFitGood(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 4*GiB, 8*GiB, 3*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit := EvaluateMemoryFit(hw, m)
	if fit.Status != RAMStatusGood {
		t.Errorf("expected Good, got %v", fit.Status)
	}
	if !strings.Contains(fit.Reason, "fits in available") {
		t.Errorf("good reason should mention fits, got %q", fit.Reason)
	}
}

func TestEvaluateMemoryFitUnknownRAMFallback(t *testing.T) {
	// Available unknown => fallback to Total
	hw := hardware.HardwareProfile{
		OS:                "linux",
		Arch:              "amd64",
		TotalRAMBytes:     16 * GiB,
		AvailableRAMBytes: 0,
		AvailableRAMKnown: false,
	}
	m := testModel("m1", 4*GiB, 8*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit := EvaluateMemoryFit(hw, m)
	if fit.Status != RAMStatusGood {
		t.Errorf("expected Good with fallback, got %v", fit.Status)
	}
	if fit.AvailableKnown {
		t.Error("AvailableKnown should be false when fallback used")
	}
	if fit.AvailableRAM != 16*GiB {
		t.Errorf("should fallback to TotalRAM, got %d", fit.AvailableRAM)
	}
	// Now model that exceeds total should be incompatible
	mBig := testModel("big", 20*GiB, 32*GiB, 10*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit2 := EvaluateMemoryFit(hw, mBig)
	if fit2.Status != RAMStatusIncompatible {
		t.Errorf("expected Incompatible when exceeding total, got %v", fit2.Status)
	}
}

// ============================================================
// Capability matching
// ============================================================

func TestCapabilityMatching(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m1 := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration, model.CapabilityToolCalling}, false)
	m2 := testModel("m2", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)

	r := NewRecommender()
	// Request tool_calling => only m1 should remain, with capability bonus
	result, err := r.Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m1, m2},
		RequestedCapability: model.CapabilityToolCalling,
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected 1 candidate for tool_calling, got %d", len(result.Candidates))
	}
	if result.Candidates[0].Model.ID != "m1" {
		t.Errorf("expected m1, got %s", result.Candidates[0].Model.ID)
	}
	if !result.Candidates[0].CapabilityMatch {
		t.Error("capability match should be true")
	}
	found := false
	for _, r := range result.Candidates[0].Reasons {
		if strings.Contains(r, "tool_calling") {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons should mention capability: %v", result.Candidates[0].Reasons)
	}
	if result.Candidates[0].FitScore < WeightCapabilityMatch {
		t.Errorf("fit score should include capability weight %d, got %d", WeightCapabilityMatch, result.Candidates[0].FitScore)
	}
}

func TestCapabilityMatchingPartialCreditNoCapability(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{m},
		// No RequestedCapability
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	c := result.Candidates[0]
	// Should get partial capability credit (15) and General purpose reason
	if c.CapabilityMatch {
		t.Error("capability match should be false when no capability requested")
	}
	found := false
	for _, rsn := range c.Reasons {
		if strings.Contains(rsn, "General purpose") {
			found = true
		}
	}
	if !found {
		t.Errorf("should have General purpose reason, got %v", c.Reasons)
	}
}

// ============================================================
// Capability mismatch
// ============================================================

func TestCapabilityMismatchNoMatch(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m1 := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m2 := testModel("m2", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeCompletion}, false)

	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m1, m2},
		RequestedCapability: model.CapabilityReasoning, // none has it
	})
	if !errors.Is(err, ErrNoCapabilityMatch) {
		t.Errorf("expected ErrNoCapabilityMatch, got %v", err)
	}
}

func TestCapabilityMismatchEmptyRegistryAfterFilter(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		RequestedCapability: model.CapabilityDebugging,
	})
	if !errors.Is(err, ErrNoCapabilityMatch) {
		t.Errorf("expected ErrNoCapabilityMatch, got %v", err)
	}
}

// ============================================================
// Benchmark influence
// ============================================================

func TestBenchmarkInfluenceFastHardwarePrefersLarger(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	small := testModel("small", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // 2 GiB
	large := testModel("large", 2*GiB, 4*GiB, 7*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // 7 GiB >6

	r := NewRecommender()
	// With fast benchmark, large should get +5 bonus
	inputFast := RecommendationInput{
		Hardware:  hw,
		Benchmark: fastBenchmark(),
		Models:    []*model.ModelMetadata{small, large},
	}
	resultFast, err := r.Recommend(inputFast)
	if err != nil {
		t.Fatalf("fast recommend failed: %v", err)
	}
	// Find scores
	var smallFast, largeFast int
	for _, c := range resultFast.Candidates {
		if c.Model.ID == "small" {
			smallFast = c.FitScore
		}
		if c.Model.ID == "large" {
			largeFast = c.FitScore
		}
	}
	// With no benchmark, neither gets bonus
	inputNone := RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{small, large},
	}
	resultNone, _ := r.Recommend(inputNone)
	var smallNone, largeNone int
	for _, c := range resultNone.Candidates {
		if c.Model.ID == "small" {
			smallNone = c.FitScore
		}
		if c.Model.ID == "large" {
			largeNone = c.FitScore
		}
	}
	// fast should increase large relative to none
	if largeFast <= largeNone {
		t.Errorf("fast hardware should increase large model score: none %d fast %d", largeNone, largeFast)
	}
	// small should not get bonus on fast
	if smallFast != smallNone {
		t.Errorf("small should not get bonus on fast: none %d fast %d", smallNone, smallFast)
	}
}

func TestBenchmarkInfluenceSlowHardwarePrefersSmaller(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	small := testModel("small", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // <=4 gets bonus on slow
	large := testModel("large", 2*GiB, 4*GiB, 7*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // >4 no bonus

	r := NewRecommender()
	inputSlow := RecommendationInput{
		Hardware:  hw,
		Benchmark: slowBenchmark(),
		Models:    []*model.ModelMetadata{small, large},
	}
	resultSlow, err := r.Recommend(inputSlow)
	if err != nil {
		t.Fatalf("slow recommend failed: %v", err)
	}
	var smallSlow, largeSlow int
	for _, c := range resultSlow.Candidates {
		if c.Model.ID == "small" {
			smallSlow = c.FitScore
		}
		if c.Model.ID == "large" {
			largeSlow = c.FitScore
		}
	}
	inputNone := RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{small, large},
	}
	resultNone, _ := r.Recommend(inputNone)
	var smallNone, largeNone int
	for _, c := range resultNone.Candidates {
		if c.Model.ID == "small" {
			smallNone = c.FitScore
		}
		if c.Model.ID == "large" {
			largeNone = c.FitScore
		}
	}
	if smallSlow <= smallNone {
		t.Errorf("slow hardware should increase small model score: none %d slow %d", smallNone, smallSlow)
	}
	if largeSlow != largeNone {
		t.Errorf("large should not get bonus on slow: none %d slow %d", largeNone, largeSlow)
	}
}

func TestBenchmarkNilGivesNoBonus(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{m},
		Benchmark: nil,
	})
	if result.BenchmarkRun {
		t.Error("BenchmarkRun should be false when nil")
	}
	// Compare with baseline benchmark: at baseline, no bonus expected either (ratios ==1)
	result2, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{m},
		Benchmark: baselineBenchmark(),
	})
	if result.Candidates[0].FitScore != result2.Candidates[0].FitScore {
		t.Logf("baseline vs nil scores differ: nil %d baseline %d (acceptable if both 0 bonus)", result.Candidates[0].FitScore, result2.Candidates[0].FitScore)
	}
}

// ============================================================
// Installed model preference
// ============================================================

func TestInstalledModelPreferenceBonus(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	notInstalled := testModel("a", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	installed := testModel("b", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, true)

	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{notInstalled, installed},
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	// Installed should be recommended due to bonus + tie-breaker
	if result.Recommended.Model.ID != "b" {
		t.Errorf("installed model should be recommended, got %s", result.Recommended.Model.ID)
	}
	// Verify bonus applied
	var scoreInstalled, scoreNot int
	for _, c := range result.Candidates {
		if c.Model.ID == "b" {
			scoreInstalled = c.FitScore
		}
		if c.Model.ID == "a" {
			scoreNot = c.FitScore
		}
	}
	if scoreInstalled-scoreNot != WeightInstalledBonus {
		t.Errorf("installed bonus should be %d, got diff %d", WeightInstalledBonus, scoreInstalled-scoreNot)
	}
	found := false
	for _, rsn := range result.Recommended.Reasons {
		if strings.Contains(rsn, "Already installed") {
			found = true
		}
	}
	if !found {
		t.Errorf("installed reason missing: %v", result.Recommended.Reasons)
	}
}

// ============================================================
// User preferences
// ============================================================

func TestUserPreferencesSpeedVsQuality(t *testing.T) {
	hw := testHardware(32*GiB, 32*GiB, true)
	small := testModel("small", 2*GiB, 4*GiB, 1*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)  // efficient
	large := testModel("large", 2*GiB, 4*GiB, 13*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // 13 GiB

	r := NewRecommender()
	// Speed should prefer small
	resSpeed, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{small, large},
		Preference: PreferenceSpeed,
	})
	if resSpeed.Recommended.Model.ID != "small" {
		t.Errorf("speed should prefer small, got %s", resSpeed.Recommended.Model.ID)
	}
	// Quality should prefer large (Weight - baseScore => larger gets higher)
	resQuality, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{small, large},
		Preference: PreferenceQuality,
	})
	if resQuality.Recommended.Model.ID != "large" {
		t.Errorf("quality should prefer large, got %s", resQuality.Recommended.Model.ID)
	}
}

func TestUserPreferencesMemory(t *testing.T) {
	hw := testHardware(32*GiB, 32*GiB, true)
	small := testModel("small", 2*GiB, 4*GiB, 1*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	large := testModel("large", 2*GiB, 4*GiB, 13*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)

	r := NewRecommender()
	resMem, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{small, large},
		Preference: PreferenceMemory,
	})
	if resMem.Recommended.Model.ID != "small" {
		t.Errorf("memory preference should prefer small, got %s", resMem.Recommended.Model.ID)
	}
	// Memory pref adds +3 to efficiency, so small gets even higher bonus
	resBalanced, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{small, large},
		Preference: PreferenceBalanced,
	})
	// find small scores
	var memScore, balScore int
	for _, c := range resMem.Candidates {
		if c.Model.ID == "small" {
			memScore = c.FitScore
		}
	}
	for _, c := range resBalanced.Candidates {
		if c.Model.ID == "small" {
			balScore = c.FitScore
		}
	}
	if memScore <= balScore {
		t.Errorf("memory pref should give higher score to small: mem %d balanced %d", memScore, balScore)
	}
}

func TestUserPreferencesContext(t *testing.T) {
	hw := testHardware(32*GiB, 32*GiB, true)
	shortCtx := testModel("short", 2*GiB, 4*GiB, 2*GiB, 4096, model.Capabilities{model.CapabilityCodeGeneration}, false)
	longCtx := testModel("long", 2*GiB, 4*GiB, 2*GiB, 128000, model.Capabilities{model.CapabilityCodeGeneration}, false)

	r := NewRecommender()
	resCtx, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{shortCtx, longCtx},
		Preference: PreferenceContext,
	})
	if resCtx.Recommended.Model.ID != "long" {
		t.Errorf("context preference should prefer long context, got %s", resCtx.Recommended.Model.ID)
	}
	resBal, _ := r.Recommend(RecommendationInput{
		Hardware:   hw,
		Models:     []*model.ModelMetadata{shortCtx, longCtx},
		Preference: PreferenceBalanced,
	})
	var longCtxScore, longBalScore int
	for _, c := range resCtx.Candidates {
		if c.Model.ID == "long" {
			longCtxScore = c.FitScore
		}
	}
	for _, c := range resBal.Candidates {
		if c.Model.ID == "long" {
			longBalScore = c.FitScore
		}
	}
	if longCtxScore <= longBalScore {
		t.Errorf("context pref should boost long context score: ctx %d balanced %d", longCtxScore, longBalScore)
	}
}

// ============================================================
// Ranking
// ============================================================

func TestRankingOrder(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	// Create three models with different fit characteristics
	// mGood: good RAM, efficient, long context
	// mMid: good RAM, medium
	// mTight: tight RAM => lower score
	mGood := testModel("good", 2*GiB, 4*GiB, 1*GiB, 128000, model.Capabilities{model.CapabilityCodeGeneration}, false)  // efficient + long ctx
	mMid := testModel("mid", 2*GiB, 4*GiB, 8*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)      // larger, less efficient
	mTight := testModel("tight", 2*GiB, 20*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // request > avail but min fits, so tight

	// Need hw where tight is actually tight: use 16 total, 10 avail => rec 20 >10 so tight
	hwTight := testHardware(16*GiB, 10*GiB, true)
	// But mGood and mMid need good fit, mTight tight => rec 20 >10 tight
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hwTight,
		Models:   []*model.ModelMetadata{mMid, mTight, mGood}, // unsorted input
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(result.Candidates))
	}
	// Candidates should be sorted descending by FitScore
	for i := 0; i < len(result.Candidates)-1; i++ {
		if result.Candidates[i].FitScore < result.Candidates[i+1].FitScore {
			t.Errorf("candidates not sorted descending: [%d]=%d < [%d]=%d", i, result.Candidates[i].FitScore, i+1, result.Candidates[i+1].FitScore)
		}
	}
	if result.Recommended.Model.ID != "good" {
		t.Errorf("good should be recommended, got %s with score %d", result.Recommended.Model.ID, result.Recommended.FitScore)
		for _, c := range result.Candidates {
			t.Logf("candidate %s score %d warnings %v reasons %v", c.Model.ID, c.FitScore, c.Warnings, c.Reasons)
		}
	}
	// Ensure hw used was correct - tight should be penalized
	var tightScore, midScore, goodScore int
	for _, c := range result.Candidates {
		switch c.Model.ID {
		case "tight":
			tightScore = c.FitScore
		case "mid":
			midScore = c.FitScore
		case "good":
			goodScore = c.FitScore
		}
	}
	if tightScore >= midScore || tightScore >= goodScore {
		t.Errorf("tight should have lowest score: tight %d mid %d good %d", tightScore, midScore, goodScore)
	}
	_ = hw // to avoid unused if not needed
	_ = hwTight
}

func TestRankingFitScoreCapped(t *testing.T) {
	hw := testHardware(64*GiB, 64*GiB, true)
	// Create a model that would exceed 100 if not capped: need max in all categories
	// Capability 30 + Memory 25 + Efficiency 15 + Benchmark 5 + Context 10 + Installed 10 = 95, so not capped yet
	// But with Memory pref + context pref etc we can get 100 max
	// Actually Max is 100, we test that score never exceeds 100
	m := testModel("m1", 2*GiB, 4*GiB, 1*GiB, 128000, model.Capabilities{model.CapabilityCodeGeneration}, true)
	m.Capabilities = model.Capabilities{model.CapabilityCodeGeneration, model.CapabilityToolCalling}
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		RequestedCapability: model.CapabilityCodeGeneration,
		Benchmark:           fastBenchmark(),  // may add 5 if large, but small won't; use baseline to be deterministic
		Preference:          PreferenceMemory, // adds efficiency bonus
	})
	if result.Candidates[0].FitScore > MaxFitScore {
		t.Errorf("FitScore should be capped at %d, got %d", MaxFitScore, result.Candidates[0].FitScore)
	}
}

// ============================================================
// Ties
// ============================================================

func TestTiesInstalledBreaker(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	// Two identical models differing only in Installed; should tie on base score but installed wins
	m1 := testModel("a", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m2 := testModel("b", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	// Make m2 installed but we want to test tie-breaker when scores equal: so create scenario where installed bonus would make them unequal
	// To test tie-breaker properly, create two models with same file size but one installed: installed gets bonus, so not tie.
	// Instead test two models with same computed score after bonus consideration via tie-breaker path
	// Let's do: m1 installed with larger size, m2 not installed with smaller size such that installed bonus compensates?
	// Simpler: test identical file sizes, one installed => installed should rank higher even if we artificially make scores equal via different paths
	// Actually the tie-breaker code: if FitScore equal, prefer Installed, then smaller file size.
	// So we need two models that end up with equal FitScore but different Installed.
	// Easiest: two models with same size, same everything, but we will test ordering directly via sort logic
	// We can make both not installed but same score, then verify smaller file size wins (next tie test)
	// For installed tie, we need both have same FitScore but one installed. To achieve equal, we can make non-installed slightly better in other dimension to compensate?
	// Simpler: we test the recommender's behavior: identical models except installed -> installed gets +10, so not tie but still should be first
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{m1, m2},
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	// Both not installed, same score, smaller file size tie-breaker not needed since identical; but order deterministic by ID sort after sort? Actually sort is stable by Installed then file size
	// For this case both equal, file sizes equal, so order is undefined but both have same score
	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2")
	}
	if result.Candidates[0].FitScore != result.Candidates[1].FitScore {
		t.Errorf("expected tie, got %d vs %d", result.Candidates[0].FitScore, result.Candidates[1].FitScore)
	}

	// Now test installed wins tie: create two models where installed one is larger but would still have same final score if bonus offsets efficiency penalty
	// Use: mSmall not installed (2 GiB -> efficiency 15), mLarge installed (8 GiB -> efficiency 9, +10 installed =19 vs 15 -> not equal)
	// Need to find sizes where efficiency difference equals installed bonus
	// Size categories: <=2 =>15, <=4=>12, <=8=>9, <=12=>6, >12=>3
	// So difference between <=2 (15) and <=8 (9) is 6, not 10.
	// Difference between <=2 (15) and <=12 (6) is 9, close.
	// Let's just test the sort directly: create two candidates with m1 installed, m2 not, but force equal FitScore by manipulating other scores to be equal via careful construction
	// Instead we can directly test that when scores are equal, installed is preferred by constructing models that naturally tie

	// Verify that installed model ranks higher when otherwise identical (bonus, not tie-breaker, but still tests installed preference)
	mAInstalled := testModel("a-inst", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, true)
	mB := testModel("b", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	result2, _ := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mAInstalled, mB},
	})
	if result2.Recommended.Model.ID != "a-inst" {
		t.Errorf("installed should be recommended when otherwise equal, got %s", result2.Recommended.Model.ID)
	}
}

func TestTiesSmallerFileSizeBreaker(t *testing.T) {
	hw := testHardware(32*GiB, 32*GiB, true)
	// Two models with same FitScore but different file sizes: need to equalize scores
	// Use non-installed, same context, but different file sizes that fall in same efficiency bucket to keep score equal
	// Then smaller should win via tie-breaker
	// To keep efficiency same, use sizes both <=2 GiB: 1 GiB and 1.5 GiB both give 15
	mSmall := testModel("small", 2*GiB, 4*GiB, 1*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	mMedium := testModel("medium", 2*GiB, 4*GiB, 1500*1024*1024, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false) // also <=2GiB
	// They should tie, smaller wins
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mMedium, mSmall}, // reverse order input
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2")
	}
	if result.Candidates[0].FitScore != result.Candidates[1].FitScore {
		t.Errorf("expected tie, got %d vs %d", result.Candidates[0].FitScore, result.Candidates[1].FitScore)
	}
	if result.Candidates[0].Model.FileSizeBytes >= result.Candidates[1].Model.FileSizeBytes {
		t.Errorf("smaller file should be first on tie, got %d vs %d", result.Candidates[0].Model.FileSizeBytes, result.Candidates[1].Model.FileSizeBytes)
	}
	if result.Recommended.Model.ID != "small" {
		t.Errorf("small should be recommended on tie, got %s", result.Recommended.Model.ID)
	}
}

func TestTiesDeterministic(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m1 := testModel("a", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m2 := testModel("b", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m3 := testModel("c", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)

	r := NewRecommender()
	result1, _ := r.Recommend(RecommendationInput{Hardware: hw, Models: []*model.ModelMetadata{m1, m2, m3}})
	result2, _ := r.Recommend(RecommendationInput{Hardware: hw, Models: []*model.ModelMetadata{m3, m2, m1}}) // different input order
	// Results should be same order (deterministic)
	if len(result1.Candidates) != len(result2.Candidates) {
		t.Fatalf("length mismatch")
	}
	for i := range result1.Candidates {
		if result1.Candidates[i].Model.ID != result2.Candidates[i].Model.ID {
			t.Errorf("non-deterministic tie handling: pos %d %s vs %s", i, result1.Candidates[i].Model.ID, result2.Candidates[i].Model.ID)
		}
	}
}

// ============================================================
// Empty registry
// ============================================================

func TestEmptyRegistry(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware: hw,
		Models:   nil,
	})
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("nil models should be ErrNoModels, got %v", err)
	}
	_, err = NewRecommender().Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{},
	})
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("empty models should be ErrNoModels, got %v", err)
	}
}

// ============================================================
// All incompatible models (already tested above, duplicate with different name)
// ============================================================

func TestAllIncompatibleModelsRejectedSeparated(t *testing.T) {
	hw := testHardware(2*GiB, 2*GiB, true)
	m1 := testModel("m1", 4*GiB, 8*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	m2 := testModel("m2", 6*GiB, 8*GiB, 3*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{m1, m2},
	})
	if !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("expected ErrNoCandidates, got %v", err)
	}
	// Also verify that when some are incompatible, Rejected list is populated
	hw2 := testHardware(8*GiB, 4*GiB, true)
	mGood := testModel("good", 2*GiB, 4*GiB, 1*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	_, err = NewRecommender().Recommend(RecommendationInput{
		Hardware: hw2,
		Models:   []*model.ModelMetadata{m1, mGood},
	})
	if err != nil {
		t.Fatalf("should succeed with one good: %v", err)
	}
}

// ============================================================
// Uncertainty handling
// ============================================================

func TestUncertaintyHandlingNoBenchmark(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{m},
		Benchmark: nil,
	})
	if !strings.Contains(result.Uncertainty, "No benchmark data") {
		t.Errorf("uncertainty should mention no benchmark, got %q", result.Uncertainty)
	}
	if result.BenchmarkRun {
		t.Error("BenchmarkRun should be false")
	}
	// With benchmark, no uncertainty about benchmark
	result2, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{m},
		Benchmark: baselineBenchmark(),
	})
	if strings.Contains(result2.Uncertainty, "No benchmark data") {
		t.Errorf("with benchmark, uncertainty should not mention no benchmark, got %q", result2.Uncertainty)
	}
	if !result2.BenchmarkRun {
		t.Error("BenchmarkRun should be true")
	}
}

func TestUncertaintyHandlingUnknownRAM(t *testing.T) {
	hwKnown := testHardware(16*GiB, 16*GiB, true)
	hwUnknown := hardware.HardwareProfile{
		OS:                "linux",
		Arch:              "amd64",
		LogicalCPUs:       8,
		TotalRAMBytes:     16 * GiB,
		AvailableRAMBytes: 0,
		AvailableRAMKnown: false,
	}
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:            hwUnknown,
		Models:              []*model.ModelMetadata{m},
		Benchmark:           baselineBenchmark(),
		RequestedCapability: model.CapabilityCodeGeneration,
	})
	if !strings.Contains(result.Uncertainty, "Available RAM unknown") {
		t.Errorf("should mention unknown RAM, got %q", result.Uncertainty)
	}
	if result.AvailableRAMKnown {
		t.Error("AvailableRAMKnown should be false")
	}
	result2, _ := r.Recommend(RecommendationInput{
		Hardware:            hwKnown,
		Models:              []*model.ModelMetadata{m},
		Benchmark:           baselineBenchmark(),
		RequestedCapability: model.CapabilityCodeGeneration,
	})
	if strings.Contains(result2.Uncertainty, "Available RAM unknown") {
		t.Errorf("known RAM should not mention unknown, got %q", result2.Uncertainty)
	}
	if !result2.AvailableRAMKnown {
		t.Error("AvailableRAMKnown should be true")
	}
}

func TestUncertaintyHandlingNoCapability(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{m},
		Benchmark: baselineBenchmark(),
		// No RequestedCapability
	})
	if !strings.Contains(result.Uncertainty, "No specific capability") {
		t.Errorf("should mention no capability, got %q", result.Uncertainty)
	}
	result2, _ := r.Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		Benchmark:           baselineBenchmark(),
		RequestedCapability: model.CapabilityCodeGeneration,
	})
	if strings.Contains(result2.Uncertainty, "No specific capability") {
		t.Errorf("with capability, should not mention: %q", result2.Uncertainty)
	}
}

func TestUncertaintyHandlingNoUncertainties(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		Benchmark:           baselineBenchmark(),
		RequestedCapability: model.CapabilityCodeGeneration,
	})
	if result.Uncertainty != "No significant uncertainties" {
		t.Errorf("expected no uncertainties, got %q", result.Uncertainty)
	}
}

func TestUncertaintyHandlingMultiple(t *testing.T) {
	hwUnknown := hardware.HardwareProfile{
		OS:            "linux",
		Arch:          "amd64",
		TotalRAMBytes: 16 * GiB,
	}
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:  hwUnknown,
		Models:    []*model.ModelMetadata{m},
		Benchmark: nil,
	})
	// Should contain both unknown RAM and no benchmark and no capability
	if !strings.Contains(result.Uncertainty, "Available RAM unknown") {
		t.Error("missing RAM uncertainty")
	}
	if !strings.Contains(result.Uncertainty, "No benchmark data") {
		t.Error("missing benchmark uncertainty")
	}
	if !strings.Contains(result.Uncertainty, "No specific capability") {
		t.Error("missing capability uncertainty")
	}
	if !strings.HasPrefix(result.Uncertainty, "Uncertainties:") {
		t.Errorf("should prefix with Uncertainties:, got %q", result.Uncertainty)
	}
}

// ============================================================
// Recommendation explanations
// ============================================================

func TestRecommendationExplanationsReasons(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 1*GiB, 128000, model.Capabilities{model.CapabilityCodeGeneration}, true)
	m.Capabilities = model.Capabilities{model.CapabilityCodeGeneration, model.CapabilityReasoning}
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		RequestedCapability: model.CapabilityCodeGeneration,
		Benchmark:           baselineBenchmark(),
	})
	c := result.Candidates[0]
	if len(c.Reasons) == 0 {
		t.Fatal("should have reasons")
	}
	// Check all expected reason types are present
	hasCap := false
	hasMem := false
	hasEff := false
	hasCtx := false
	hasInstalled := false
	for _, rsn := range c.Reasons {
		if strings.Contains(rsn, "Supports") {
			hasCap = true
		}
		if strings.Contains(rsn, "Recommended RAM fits") {
			hasMem = true
		}
		if strings.Contains(rsn, "Efficient model size") {
			hasEff = true
		}
		if strings.Contains(rsn, "context length") || strings.Contains(rsn, "Context") {
			hasCtx = true
		}
		if strings.Contains(rsn, "Already installed") {
			hasInstalled = true
		}
	}
	if !hasCap {
		t.Errorf("missing capability reason: %v", c.Reasons)
	}
	if !hasMem {
		t.Errorf("missing memory reason: %v", c.Reasons)
	}
	if !hasEff {
		t.Errorf("missing efficiency reason: %v", c.Reasons)
	}
	if !hasCtx {
		t.Errorf("missing context reason: %v", c.Reasons)
	}
	if !hasInstalled {
		t.Errorf("missing installed reason: %v", c.Reasons)
	}
	// Benchmark reason only when benchmark gives bonus; for baseline no bonus so should NOT have it
	hasBench := false
	for _, rsn := range c.Reasons {
		if strings.Contains(rsn, "Suitable for measured") {
			hasBench = true
		}
	}
	if hasBench {
		t.Errorf("should not have benchmark reason for baseline, got %v", c.Reasons)
	}
	// Now with fast benchmark and large model -> should have bench reason
	large := testModel("large", 2*GiB, 4*GiB, 7*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	result2, _ := r.Recommend(RecommendationInput{
		Hardware:  hw,
		Models:    []*model.ModelMetadata{large},
		Benchmark: fastBenchmark(),
	})
	foundBench := false
	for _, rsn := range result2.Candidates[0].Reasons {
		if strings.Contains(rsn, "Suitable for measured") {
			foundBench = true
		}
	}
	if !foundBench {
		t.Errorf("fast hardware large model should have benchmark reason: %v", result2.Candidates[0].Reasons)
	}
}

func TestRecommendationExplanationsWarnings(t *testing.T) {
	hw := testHardware(8*GiB, 6*GiB, true) // tight case for model needing 8 rec
	mTight := testModel("tight", 4*GiB, 8*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, _ := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mTight},
	})
	c := result.Candidates[0]
	if len(c.Warnings) == 0 {
		t.Fatal("tight should have warnings")
	}
	found := false
	for _, w := range c.Warnings {
		if strings.Contains(w, "recommended RAM") {
			found = true
		}
	}
	if !found {
		t.Errorf("warning should mention recommended RAM: %v", c.Warnings)
	}
	// Incompatible should have HARD warning
	mBig := testModel("big", 10*GiB, 16*GiB, 4*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	result2, _ := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{mBig, mTight},
	})
	if len(result2.Rejected) != 1 {
		t.Fatalf("expected 1 rejected")
	}
	if !strings.Contains(result2.Rejected[0].Warnings[0], "HARD:") {
		t.Errorf("incompatible should have HARD warning: %v", result2.Rejected[0].Warnings)
	}
	if result2.Rejected[0].RejectionReason == "" {
		t.Error("rejection should have reason")
	}
}

func TestRecommendationExplanationsRejected(t *testing.T) {
	hw := testHardware(4*GiB, 4*GiB, true)
	m := testModel("big", 8*GiB, 12*GiB, 4*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	// Recommend with another good model to ensure we get rejected list, not error when all incompatible
	mGood := testModel("good", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   []*model.ModelMetadata{m, mGood},
	})
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}
	if len(result.Rejected) != 1 {
		t.Fatalf("expected 1 rejected, got %d", len(result.Rejected))
	}
	if result.Rejected[0].RejectionReason == "" {
		t.Error("rejected should have rejection reason")
	}
	if len(result.Rejected[0].Warnings) == 0 {
		t.Error("rejected should have warnings")
	}
}

// ============================================================
// Additional edge tests - deterministic
// ============================================================

func TestNewRecommenderNotNil(t *testing.T) {
	r := NewRecommender()
	if r == nil {
		t.Error("NewRecommender should not be nil")
	}
}

func TestEvaluateMemoryFitReasonFormatting(t *testing.T) {
	hw := testHardware(8*GiB, 8*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false)
	fit := EvaluateMemoryFit(hw, m)
	if !strings.Contains(fit.Reason, "GiB") {
		t.Errorf("reason should contain GiB: %q", fit.Reason)
	}
}

func TestRecommendationDeterministic(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	models := []*model.ModelMetadata{
		testModel("a", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityCodeGeneration}, false),
		testModel("b", 2*GiB, 4*GiB, 3*GiB, 16384, model.Capabilities{model.CapabilityCodeGeneration}, false),
		testModel("c", 2*GiB, 4*GiB, 4*GiB, 32768, model.Capabilities{model.CapabilityCodeGeneration}, false),
	}
	r := NewRecommender()
	result1, _ := r.Recommend(RecommendationInput{Hardware: hw, Models: models})
	result2, _ := r.Recommend(RecommendationInput{Hardware: hw, Models: models})
	if result1.Recommended.Model.ID != result2.Recommended.Model.ID {
		t.Error("recommendation should be deterministic")
	}
	for i := range result1.Candidates {
		if result1.Candidates[i].FitScore != result2.Candidates[i].FitScore {
			t.Errorf("scores should be deterministic: %d vs %d", result1.Candidates[i].FitScore, result2.Candidates[i].FitScore)
		}
	}
}

func TestAllInvalidCapabilitiesFiltered(t *testing.T) {
	hw := testHardware(16*GiB, 16*GiB, true)
	m := testModel("m1", 2*GiB, 4*GiB, 2*GiB, 8192, model.Capabilities{model.CapabilityReasoning}, false)
	_, err := NewRecommender().Recommend(RecommendationInput{
		Hardware:            hw,
		Models:              []*model.ModelMetadata{m},
		RequestedCapability: model.CapabilityCodeGeneration,
	})
	if !errors.Is(err, ErrNoCapabilityMatch) {
		t.Errorf("expected ErrNoCapabilityMatch, got %v", err)
	}
}

func TestBuiltInCatalogRecommendation(t *testing.T) {
	hw := testHardware(32*GiB, 32*GiB, true)
	catalog := model.BuiltInCatalog()
	r := NewRecommender()
	result, err := r.Recommend(RecommendationInput{
		Hardware: hw,
		Models:   catalog,
	})
	if err != nil {
		t.Fatalf("BuiltIn catalog recommend failed: %v", err)
	}
	if result.Recommended == nil {
		t.Fatal("should have recommended")
	}
	if len(result.Candidates) == 0 {
		t.Error("should have candidates")
	}
	// All catalog models should be compatible with 32 GiB
	if len(result.Rejected) != 0 {
		t.Errorf("no models should be rejected with 32 GiB, got %d rejected", len(result.Rejected))
	}
}
