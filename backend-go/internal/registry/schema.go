package registry

import (
	"fmt"
	"time"
)

type Tier string
type HealthStatus string
type FreshnessState string

const (
	TierFrontier  Tier = "frontier"
	TierFast      Tier = "fast"
	TierEconomy   Tier = "economy"
	TierReasoning Tier = "reasoning"

	HealthHealthy  HealthStatus = "HEALTHY"
	HealthDegraded HealthStatus = "DEGRADED"
	HealthDown     HealthStatus = "DOWN"

	FreshnessFresh   FreshnessState = "FRESH"
	FreshnessStale   FreshnessState = "STALE"
	FreshnessExpired FreshnessState = "EXPIRED"
)

type PublishedMetrics struct {
	ContextWindow       int     `json:"context_window" bson:"context_window"`
	InputCostPerMToken  float64 `json:"input_cost_per_mtoken" bson:"input_cost_per_mtoken"`
	OutputCostPerMToken float64 `json:"output_cost_per_mtoken" bson:"output_cost_per_mtoken"`
	EloRating           int     `json:"elo_rating" bson:"elo_rating"`
}

type ObservedMetrics struct {
	P50LatencyMs   int          `json:"p50_latency_ms" bson:"p50_latency_ms"`
	P95LatencyMs   int          `json:"p95_latency_ms" bson:"p95_latency_ms"`
	OutputSpeedTPS float64      `json:"output_speed_tps" bson:"output_speed_tps"`
	ErrorRate5m    float64      `json:"error_rate_5m" bson:"error_rate_5m"`
	HealthStatus   HealthStatus `json:"health_status" bson:"health_status"`
}

type Provenance struct {
	SourceAPIs       []string       `json:"source_apis" bson:"source_apis"`
	SourceTrustScore float64        `json:"source_trust_score" bson:"source_trust_score"`
	FetchedAt        time.Time      `json:"fetched_at" bson:"fetched_at"`
	SnapshotVersion  int            `json:"snapshot_version" bson:"snapshot_version"`
	FreshnessState   FreshnessState `json:"freshness_state" bson:"freshness_state"`
}

type ModelFact struct {
	Provider  string           `json:"provider" bson:"provider"`
	ModelID   string           `json:"model_id" bson:"model_id"`
	Tier      Tier             `json:"tier" bson:"tier"`
	Published PublishedMetrics `json:"published" bson:"published"`
	Observed  ObservedMetrics  `json:"observed" bson:"observed"`
	Provenance Provenance       `json:"provenance" bson:"provenance"`
}

// Validate checks the boundary conditions of a ModelFact.
func (mf *ModelFact) Validate() error {
	if mf.Published.ContextWindow < 1024 || mf.Published.ContextWindow > 10000000 {
		return fmt.Errorf("context_window %d out of bounds [1024, 10000000]", mf.Published.ContextWindow)
	}
	if mf.Published.InputCostPerMToken < 0.0001 || mf.Published.InputCostPerMToken > 1000.0 {
		return fmt.Errorf("input_cost_per_mtoken %f out of bounds [0.0001, 1000.0]", mf.Published.InputCostPerMToken)
	}
	if mf.Published.OutputCostPerMToken < 0.0001 || mf.Published.OutputCostPerMToken > 1000.0 {
		return fmt.Errorf("output_cost_per_mtoken %f out of bounds [0.0001, 1000.0]", mf.Published.OutputCostPerMToken)
	}
	return nil
}
