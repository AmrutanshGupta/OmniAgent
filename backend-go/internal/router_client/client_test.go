package router_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
)

func TestRouteTask_Success(t *testing.T) {
	mockResp := DynamicRoutingResponse{
		SelectedModel:   "anthropic/claude-3-5-sonnet",
		ExpectedUtility: 0.92,
		MetricsDecomposition: MetricsDecomposition{
			QualityScore:      0.95,
			NormalizedCost:    0.20,
			NormalizedLatency: 0.15,
			ReliabilityScore:  0.99,
		},
		RankedAlternatives: []string{"openai/gpt-4o-mini"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/route" {
			http.NotFound(w, r)
			return
		}
		var req DynamicRoutingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer server.Close()

	cfg := &config.Config{PythonRouterURL: server.URL}

	weights := SLAWeights{WQ: 1.0, WC: 1.0, WL: 1.0, WR: 1.0}
	candidates := []registry.ModelFact{}

	resp, err := RouteTask(context.Background(), "Write a sorting algorithm", weights, candidates, cfg)
	if err != nil {
		t.Fatalf("RouteTask failed: %v", err)
	}
	if resp.SelectedModel != "anthropic/claude-3-5-sonnet" {
		t.Errorf("expected claude-3-5-sonnet, got %s", resp.SelectedModel)
	}
	if resp.ExpectedUtility != 0.92 {
		t.Errorf("expected utility 0.92, got %f", resp.ExpectedUtility)
	}
}

func TestGetOptimalRoute_FallbackWhenOffline(t *testing.T) {
	cfg := &config.Config{PythonRouterURL: "http://127.0.0.1:59999"} // Unreachable port

	req := models.RouterRequest{
		PromptTokens: 100,
		WQ:           1.0,
		WC:           1.0,
		WL:           1.0,
	}

	candidates := []registry.ModelFact{}
	resp, err := GetOptimalRoute(req, cfg, candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ChosenAgentID != 2 {
		t.Errorf("expected fallback ChosenAgentID=2, got %d", resp.ChosenAgentID)
	}
}
