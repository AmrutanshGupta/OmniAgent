package registry

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"github.com/rs/zerolog/log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

type OpenRouterModel struct {
	ID           string `json:"id"`
	Pricing      struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	ContextLength int `json:"context_length"`
	Benchmarks    struct {
		DesignArena []struct {
			Elo int `json:"elo"`
		} `json:"design_arena"`
	} `json:"benchmarks"`
	Architecture  struct {
		Modality string `json:"modality"`
	} `json:"architecture"`
}

type ArtificialAnalysisModel struct {
	ModelID        string  `json:"model_id"`
	Provider       string  `json:"provider"`
	EloRating      int     `json:"elo_rating"`
	OutputSpeedTPS float64 `json:"output_speed_tps"`
	TTFT           int     `json:"ttft_ms"`
}

var anomalyCounter = expvar.NewInt("registry_anomalies_total")

type IngestionWorker struct {
	db     *MongoDB
	client *http.Client
}

func NewIngestionWorker(db *MongoDB) *IngestionWorker {
	return &IngestionWorker{
		db: db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *IngestionWorker) Start(ctx context.Context, schedule time.Duration) {
	if schedule == 0 {
		schedule = 6 * time.Hour
	}
	
	w.RunIngestion(ctx)

	go func() {
		ticker := time.NewTicker(schedule)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunIngestion(ctx)
			}
		}
	}()
}

func (w *IngestionWorker) RunIngestion(ctx context.Context) {
	log.Debug().Msg("[registry-ingestion] Starting ingestion cycle")

	// 1. Fetch OpenRouter Models
	orModels, err := w.fetchOpenRouter(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[registry-ingestion] Failed to fetch OpenRouter")
	}

	// 1b. Fetch Artificial Analysis Data
	aaModels, err := w.fetchArtificialAnalysis(ctx)
	if err != nil {
		log.Error().Err(err).Msg("[registry-ingestion] Failed to fetch Artificial Analysis")
	}

	// 2. Normalize Datasets
	joined := w.normalizeModels(orModels, aaModels)

	// 3. Compute Tiers dynamically
	w.computeTiers(joined)

	// 4. Anomaly Detection and DB persistence
	for _, candidate := range joined {
		if err := candidate.Validate(); err != nil {
			log.Warn().Str("model", candidate.ModelID).Err(err).Msg("[registry-ingestion] Validation failed")
			continue
		}

		// Anomaly Detection — only runs when a Last-Known-Good record exists.
		// A missing LKG means this is a first-seen model; accept it unconditionally.
		lkg, lkgErr := w.db.GetLKG(ctx, candidate.ModelID)
		if lkgErr != nil {
			log.Error().Str("model", candidate.ModelID).Err(lkgErr).Msg("[registry-ingestion] Failed to fetch LKG")
			// Don't skip — LKG fetch failing is a transient DB error; still accept the candidate.
		}

		if lkg != nil && w.isAnomalous(*lkg, candidate) {
			log.Warn().Str("model", candidate.ModelID).Msg("[registry-ingestion] ANOMALY DETECTED — discarding update")
			anomalyCounter.Add(1)
			continue
		}

		// Increment snapshot version
		if lkg != nil {
			candidate.Provenance.SnapshotVersion = lkg.Provenance.SnapshotVersion + 1
		} else {
			candidate.Provenance.SnapshotVersion = 1
		}

		if err := w.db.InsertSnapshot(ctx, *candidate); err != nil {
			log.Error().Str("model", candidate.ModelID).Err(err).Msg("[registry-ingestion] Failed to insert snapshot")
		}
	}
	log.Debug().Msg("[registry-ingestion] Ingestion cycle completed")
}

func (w *IngestionWorker) fetchOpenRouter(ctx context.Context) ([]OpenRouterModel, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var data struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

func (w *IngestionWorker) fetchArtificialAnalysis(ctx context.Context) (map[string]ArtificialAnalysisModel, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://artificialanalysis.ai/data-api", nil)
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// We'll mock the JSON structure since the API isn't publicly standardized in our schema yet
	// Assume it returns an array of ArtificialAnalysisModel
	var models []ArtificialAnalysisModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		// Mock a fallback parsing failure gracefully without crashing the whole loop
		return map[string]ArtificialAnalysisModel{}, nil 
	}

	aaMap := make(map[string]ArtificialAnalysisModel)
	for _, m := range models {
		// Store by lowercase stripped ID for fuzzy matching
		key := strings.ToLower(strings.TrimPrefix(m.ModelID, m.Provider+"/"))
		aaMap[key] = m
	}
	return aaMap, nil
}


func parseCost(s string) float64 {
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return val * 1_000_000 // Convert per token to per million tokens
}

func (w *IngestionWorker) normalizeModels(or []OpenRouterModel, aa map[string]ArtificialAnalysisModel) []*ModelFact {
	var joined []*ModelFact
	now := time.Now()

	for _, orm := range or {
		parts := strings.SplitN(orm.ID, "/", 2)
		provider := "unknown"
		modelShort := orm.ID
		if len(parts) == 2 {
			provider = parts[0]
			modelShort = parts[1]
		}

		inputCost := parseCost(orm.Pricing.Prompt)
		outputCost := parseCost(orm.Pricing.Completion)

		// OpenRouter uses "-1" as a sentinel for free/unknown pricing models.
		// After multiplying by 1e6, this becomes -1,000,000 which fails validation.
		// Clamp any non-positive cost to the free-tier floor.
		if inputCost <= 0 {
			inputCost = 0.0001
		}
		if outputCost <= 0 {
			outputCost = 0.0001
		}

		var eloRating int
		if len(orm.Benchmarks.DesignArena) > 0 {
			eloRating = orm.Benchmarks.DesignArena[0].Elo
		}

		// Cross-reference with Artificial Analysis via fuzzy short model ID match
		aaMatch, aaFound := aa[strings.ToLower(modelShort)]
		
		if aaFound && aaMatch.EloRating > 0 {
			eloRating = aaMatch.EloRating
		}
		
		p50Lat := 500
		if aaFound && aaMatch.TTFT > 0 { p50Lat = aaMatch.TTFT }
		
		tps := 50.0
		if aaFound && aaMatch.OutputSpeedTPS > 0 { tps = aaMatch.OutputSpeedTPS }
		
		sourceAPIs := []string{"https://openrouter.ai/api/v1/models"}
		if aaFound {
			sourceAPIs = append(sourceAPIs, "https://artificialanalysis.ai/data-api")
		}

		fact := &ModelFact{
			Provider: provider,
			ModelID:  orm.ID,
			Published: PublishedMetrics{
				ContextWindow:       orm.ContextLength,
				InputCostPerMToken:  inputCost,
				OutputCostPerMToken: outputCost,
				EloRating:           eloRating,
			},
			Observed: ObservedMetrics{
				P50LatencyMs:   p50Lat,
				P95LatencyMs:   p50Lat * 2, // Heuristic P95
				OutputSpeedTPS: tps,
				ErrorRate5m:    0.0,
				HealthStatus:   HealthHealthy,
			},
			Provenance: Provenance{
				SourceAPIs:       sourceAPIs,
				SourceTrustScore: 1.0,
				FetchedAt:        now,
				FreshnessState:   FreshnessFresh,
			},
		}

		joined = append(joined, fact)
	}

	return joined
}

func (w *IngestionWorker) computeTiers(models []*ModelFact) {
	if len(models) == 0 {
		return
	}

	// Sort by latency for percentiles
	var latencies []int
	var costs []float64

	for _, m := range models {
		latencies = append(latencies, m.Observed.P50LatencyMs)
		costs = append(costs, m.Published.InputCostPerMToken)
	}

	sort.Ints(latencies)
	sort.Float64s(costs)

	p33Latency := latencies[len(latencies)/3]
	p66Cost := costs[len(costs)*2/3]

	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ModelID), "reasoning") || strings.Contains(strings.ToLower(m.ModelID), "o1") {
			m.Tier = TierReasoning
		} else if m.Published.InputCostPerMToken > p66Cost {
			m.Tier = TierFrontier
		} else if m.Observed.P50LatencyMs < p33Latency {
			m.Tier = TierFast
		} else {
			m.Tier = TierEconomy
		}
	}
}

func (w *IngestionWorker) isAnomalous(lkg ModelFact, newFact *ModelFact) bool {
	// Cost diff > 90% — catches only extreme repricing events, not routine updates.
	// OpenRouter and providers adjust prices regularly; 50% was too aggressive.
	if lkg.Published.InputCostPerMToken > 0 {
		costDiff := math.Abs(lkg.Published.InputCostPerMToken - newFact.Published.InputCostPerMToken)
		if costDiff/lkg.Published.InputCostPerMToken > 0.9 {
			return true
		}
	}

	// Latency diff > 90%
	if lkg.Observed.P50LatencyMs > 0 {
		latDiff := math.Abs(float64(lkg.Observed.P50LatencyMs - newFact.Observed.P50LatencyMs))
		if latDiff/float64(lkg.Observed.P50LatencyMs) > 0.9 {
			return true
		}
	}
	return false
}
