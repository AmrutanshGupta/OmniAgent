package registry

import (
	"testing"
)

func TestModelFactValidate(t *testing.T) {
	fact := ModelFact{
		Published: PublishedMetrics{
			ContextWindow:       100000,
			InputCostPerMToken:  0.5,
			OutputCostPerMToken: 1.5,
		},
	}

	if err := fact.Validate(); err != nil {
		t.Errorf("Expected valid fact, got error: %v", err)
	}

	// Test boundary conditions
	fact.Published.ContextWindow = 500 // Too small
	if err := fact.Validate(); err == nil {
		t.Errorf("Expected error for small context_window")
	}

	fact.Published.ContextWindow = 100000
	fact.Published.InputCostPerMToken = 0.00001 // Too cheap
	if err := fact.Validate(); err == nil {
		t.Errorf("Expected error for small cost")
	}
}

func TestAnomalyDetection(t *testing.T) {
	worker := &IngestionWorker{}

	lkg := ModelFact{
		Published: PublishedMetrics{InputCostPerMToken: 1.0},
		Observed:  ObservedMetrics{P50LatencyMs: 100},
	}

	// Safe increase (20%)
	newFact := &ModelFact{
		Published: PublishedMetrics{InputCostPerMToken: 1.2},
		Observed:  ObservedMetrics{P50LatencyMs: 120},
	}
	if worker.isAnomalous(lkg, newFact) {
		t.Errorf("Expected safe increase to not be anomalous")
	}

	// Cost Anomaly (> 90%)
	newFact2 := &ModelFact{
		Published: PublishedMetrics{InputCostPerMToken: 2.0},
		Observed:  ObservedMetrics{P50LatencyMs: 100},
	}
	if !worker.isAnomalous(lkg, newFact2) {
		t.Errorf("Expected cost anomaly to be detected")
	}

	// Latency Anomaly (> 90%)
	newFact3 := &ModelFact{
		Published: PublishedMetrics{InputCostPerMToken: 1.0},
		Observed:  ObservedMetrics{P50LatencyMs: 200},
	}
	if !worker.isAnomalous(lkg, newFact3) {
		t.Errorf("Expected latency anomaly to be detected")
	}
}

func TestNormalizeModels(t *testing.T) {
	worker := &IngestionWorker{}

	or := []OpenRouterModel{
		{
			ID: "openai/gpt-4o",
			Pricing: struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			}{"0.0000025", "0.00001"},
			ContextLength: 128000,
			Benchmarks: struct {
				DesignArena []struct {
					Elo int `json:"elo"`
				} `json:"design_arena"`
			}{
				DesignArena: []struct {
					Elo int `json:"elo"`
				}{{Elo: 1250}},
			},
		},
	}

	joined := worker.normalizeModels(or, map[string]ArtificialAnalysisModel{})

	if len(joined) != 1 {
		t.Fatalf("Expected 1 joined model, got %d", len(joined))
	}
	
	j := joined[0]
	
	if j.ModelID != "openai/gpt-4o" {
		t.Errorf("Expected model ID openai/gpt-4o, got %s", j.ModelID)
	}
	if j.Provider != "openai" {
		t.Errorf("Expected provider openai, got %s", j.Provider)
	}
	if j.Observed.OutputSpeedTPS != 50.0 {
		t.Errorf("Expected OutputSpeedTPS 50.0, got %f", j.Observed.OutputSpeedTPS)
	}
	if j.Observed.P50LatencyMs != 500 {
		t.Errorf("Expected P50LatencyMs 500, got %d", j.Observed.P50LatencyMs)
	}
	if j.Published.InputCostPerMToken != 2.5 {
		t.Errorf("Expected InputCostPerMToken 2.5, got %f", j.Published.InputCostPerMToken)
	}
	if j.Published.EloRating != 1250 {
		t.Errorf("Expected EloRating 1250, got %d", j.Published.EloRating)
	}
}
