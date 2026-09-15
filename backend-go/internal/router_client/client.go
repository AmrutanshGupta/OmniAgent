// backend-go/internal/router_client/client.go
package router_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"
	"strings"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
	"go.opentelemetry.io/otel"
)

type SLAWeights struct {
	WQ float64 `json:"w_q"`
	WC float64 `json:"w_c"`
	WL float64 `json:"w_l"`
	WR float64 `json:"w_r"`
}

type MetricsDecomposition struct {
	QualityScore      float64 `json:"quality_score"`
	NormalizedCost    float64 `json:"normalized_cost"`
	NormalizedLatency float64 `json:"normalized_latency"`
	ReliabilityScore  float64 `json:"reliability_score"`
}

type DynamicRoutingRequest struct {
	Prompt          string               `json:"prompt"`
	SLAWeights      SLAWeights           `json:"sla_weights"`
	CandidateModels []registry.ModelFact `json:"candidate_models"`
}

type DynamicRoutingResponse struct {
	SelectedModel        string               `json:"selected_model"`
	ExpectedUtility      float64              `json:"expected_utility"`
	MetricsDecomposition MetricsDecomposition `json:"metrics_decomposition"`
	RankedAlternatives   []string             `json:"ranked_alternatives"`
}

// RouteTask queries router-ml's dynamic /v1/route endpoint using live Model Registry snapshots.
func RouteTask(ctx context.Context, prompt string, weights SLAWeights, candidates []registry.ModelFact, cfg *config.Config) (*DynamicRoutingResponse, error) {
	baseURL := cfg.PythonRouterURL
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	url := baseURL + "/v1/route"

	reqBody := DynamicRoutingRequest{
		Prompt:          prompt,
		SLAWeights:      weights,
		CandidateModels: candidates,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("router-ml returned status %d", resp.StatusCode)
	}

	var routeResp DynamicRoutingResponse
	if err := json.NewDecoder(resp.Body).Decode(&routeResp); err != nil {
		return nil, err
	}

	return &routeResp, nil
}

// GetOptimalRoute adapts the legacy RouterRequest to the new dynamic ML router endpoint.
func GetOptimalRoute(req models.RouterRequest, cfg *config.Config, candidates []registry.ModelFact) (*models.RouterResponse, error) {
	tracer := otel.Tracer("router_client")
	_, span := tracer.Start(context.Background(), "GetOptimalRoute")
	defer span.End()

	baseURL := cfg.PythonRouterURL
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	url := baseURL + "/v1/route"

	filteredCandidates := FilterCandidates(candidates, req)
	if len(filteredCandidates) == 0 {
		log.Warn().Msg("[router_client] All candidates rejected by policy, falling back to Tier 2")
		return &models.RouterResponse{ChosenAgentID: 2}, nil
	}

	// Construct dynamic request
	dynamicReq := DynamicRoutingRequest{
		Prompt: "Task execution request",
		SLAWeights: SLAWeights{
			WQ: req.WQ,
			WC: req.WC,
			WL: req.WL,
			WR: 1.0,
		},
		CandidateModels: filteredCandidates,
	}

	payload, err := json.Marshal(dynamicReq)
	if err != nil {
		log.Warn().Err(err).Msg("[router_client] Payload marshal error, falling back")
		return &models.RouterResponse{ChosenAgentID: 2}, nil
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Warn().Err(err).Msg("[router_client] Connection error, falling back to Tier 2")
		return &models.RouterResponse{ChosenAgentID: 2}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("[router_client] Bad status, falling back")
		return &models.RouterResponse{ChosenAgentID: 2}, nil
	}

	var dynResp DynamicRoutingResponse
	if err := json.NewDecoder(resp.Body).Decode(&dynResp); err != nil {
		log.Warn().Err(err).Msg("[router_client] Decode error, falling back")
		return &models.RouterResponse{ChosenAgentID: 2}, nil
	}

	// Map selected model string to ChosenAgentID
	chosenAgent := 2 // default to flash
	modelLower := strings.ToLower(dynResp.SelectedModel)
	if strings.Contains(modelLower, "gpt") || strings.Contains(modelLower, "openai") {
		chosenAgent = 0
	} else if strings.Contains(modelLower, "sonnet") || strings.Contains(modelLower, "claude") {
		chosenAgent = 1
	}

	return &models.RouterResponse{
		ChosenAgentID:   chosenAgent,
		ExpectedLatency: dynResp.MetricsDecomposition.NormalizedLatency,
		ExpectedCost:    dynResp.MetricsDecomposition.NormalizedCost,
	}, nil
}