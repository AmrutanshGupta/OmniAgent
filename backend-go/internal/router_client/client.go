package router_client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type RouteRequest struct {
	PromptTokens int     `json:"promptTokens"`
	Complexity   float64 `json:"complexity"`
	QueueDepth   int     `json:"queueDepth"`
	QualityBias  float64 `json:"qualityBias"` // from SLA slider
}

type RouteResponse struct {
	Tier       string  `json:"tier"`
	Confidence float64 `json:"confidence"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func routerURL() string {
	if u := os.Getenv("ROUTER_ML_URL"); u != "" {
		return u
	}
	return "http://localhost:8000"
}

// Route calls the FastAPI/PyTorch neural router. Falls back to a local
// heuristic if the Python service is unreachable so the Go orchestrator
// never hard-fails on a missing sidecar.
func Route(req RouteRequest) RouteResponse {
	body, _ := json.Marshal(req)
	resp, err := httpClient.Post(routerURL()+"/route", "application/json", bytes.NewReader(body))
	if err != nil {
		return localFallback(req)
	}
	defer resp.Body.Close()
	var out RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return localFallback(req)
	}
	return out
}

func localFallback(req RouteRequest) RouteResponse {
	score := req.Complexity*0.6 + float64(req.QueueDepth)*0.02 - req.QualityBias*0.3
	switch {
	case score < 0.35:
		return RouteResponse{Tier: "flash", Confidence: 0.5}
	case score < 0.7:
		return RouteResponse{Tier: "sonnet", Confidence: 0.5}
	default:
		return RouteResponse{Tier: "gpt-4o", Confidence: 0.5}
	}
}
