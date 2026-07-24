package agents

import (
	"fmt"

	"llm-orchestrator/backend-go/internal/llm_clients"
	"llm-orchestrator/backend-go/internal/models"
	"llm-orchestrator/backend-go/internal/router_client"
)

var systemPrompts = map[string]string{
	"coding":     "You are a precise senior software engineer. Return only correct, runnable code plus a one-line explanation.",
	"data":       "You are a data analyst. Be quantitative and cite concrete numbers/structure in your answer.",
	"creative":   "You are a creative copywriter. Be vivid but concise.",
	"synthesis":  "You merge multiple specialist outputs into one coherent final answer.",
}

// ExecuteTask asks the infra-aware Router for a model tier (based on live
// queue depth + prompt complexity + SLA bias), then runs the task at that
// tier via the LLM client. It does not validate quality — that's the
// Critic's job (see critic_cascade.go).
func ExecuteTask(t *models.Task, queueDepth int, qualityBias float64, keys models.APIKeyBundle) {
	t.Status = models.StatusExecuting
	t.Attempts++

	route := router_client.Route(router_client.RouteRequest{
		PromptTokens: len(t.Description) / 4,
		Complexity:   complexityOf(t.Description),
		QueueDepth:   queueDepth,
		QualityBias:  qualityBias,
	})
	t.Tier = route.Tier

	// Respect the FrugalGPT cascade position if the Critic already escalated us.
	if t.CascadeIdx > 0 && t.CascadeIdx < len(t.Cascade) {
		t.Tier = t.Cascade[t.CascadeIdx]
	}

	sys := systemPrompts[t.Domain]
	if sys == "" {
		sys = "You are a helpful assistant."
	}

	result, err := llm_clients.Complete(t.Tier, sys, t.Description, keys.OpenAI, keys.Anthropic, keys.Google)
	if err != nil {
		t.Status = models.StatusFailed
		t.Output = fmt.Sprintf("error: %v", err)
		return
	}
	t.Output = result.Text
	t.TokensUsed = result.TokensUsed
	t.CostUSD = estimateCost(t.Tier, result.TokensUsed)
	t.Status = models.StatusCompleted
}

func complexityOf(desc string) float64 {
	l := len(desc)
	switch {
	case l < 80:
		return 0.2
	case l < 200:
		return 0.5
	default:
		return 0.85
	}
}

// estimateCost uses rough public per-1K-token pricing for telemetry display only.
func estimateCost(tier string, tokens int) float64 {
	perK := map[string]float64{"flash": 0.0004, "sonnet": 0.003, "gpt-4o": 0.005}
	rate, ok := perK[tier]
	if !ok {
		rate = 0.002
	}
	return (float64(tokens) / 1000.0) * rate
}
