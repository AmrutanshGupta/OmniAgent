package router_client

import (
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
)

// FilterCandidates applies deterministic constraints to the candidate models before routing.
func FilterCandidates(candidates []registry.ModelFact, req models.RouterRequest) []registry.ModelFact {
	var filtered []registry.ModelFact

	// Minimum context window required (20% margin)
	minContext := int(float64(req.PromptTokens) * 1.2)

	for _, m := range candidates {
		// 1. Health constraint
		if m.Observed.HealthStatus == registry.HealthDown {
			continue
		}

		// 2. Context window constraint
		if m.Published.ContextWindow > 0 && m.Published.ContextWindow < minContext {
			continue
		}

		// (Future: Add capability matching, budget caps, privacy rules, etc. here)

		filtered = append(filtered, m)
	}

	return filtered
}
