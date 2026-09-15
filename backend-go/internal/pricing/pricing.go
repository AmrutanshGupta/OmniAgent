package pricing

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"net/http"
	"sync"
	"time"
)

type ModelPricing struct {
	InputCost  float64 `json:"input_cost_per_token"`
	OutputCost float64 `json:"output_cost_per_token"`
}

var (
	cache map[string]ModelPricing
	mu    sync.RWMutex
)

// InitPricingCache fetches real-time model prices and starts a daily refresh ticker.
func InitPricingCache() {
	cache = make(map[string]ModelPricing)
	updateCache()

	// Background goroutine to refresh prices every 24 hours
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			updateCache()
		}
	}()
}

func updateCache() {
	// LiteLLM's centralized dynamic pricing repository
	url := "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	
	resp, err := http.Get(url)
	if err != nil {
		log.Warn().Err(err).Msg("[pricing] Failed to fetch dynamic pricing")
		return
	}
	defer resp.Body.Close()

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		log.Warn().Err(err).Msg("[pricing] Failed to decode pricing JSON")
		return
	}

	newCache := make(map[string]ModelPricing)
	
	for key, value := range raw {
		if modelData, ok := value.(map[string]interface{}); ok {
			var p ModelPricing
			if ic, ok := modelData["input_cost_per_token"].(float64); ok {
				p.InputCost = ic
			}
			if oc, ok := modelData["output_cost_per_token"].(float64); ok {
				p.OutputCost = oc
			}
			newCache[key] = p
		}
	}

	mu.Lock()
	cache = newCache
	mu.Unlock()
	log.Info().Msg("[pricing] Dynamic pricing cache updated")
}

// GetOutputCost fetches the cost from the live cache, or returns a safe fallback if missing.
func GetOutputCost(modelName string, fallbackCost float64) float64 {
	mu.RLock()
	defer mu.RUnlock()
	
	if p, exists := cache[modelName]; exists && p.OutputCost > 0 {
		return p.OutputCost
	}
	
	return fallbackCost
}