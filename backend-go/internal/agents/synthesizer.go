package agents

import (
	"strings"

	"llm-orchestrator/backend-go/internal/models"
)

// Synthesize compiles all completed task outputs into one final payload.
// Pure Go string composition — no LLM calls, matching the PRD's "native Go
// logic (zero LLM calls)" requirement.
func Synthesize(tasks []models.Task) string {
	var b strings.Builder
	b.WriteString("# Final Synthesized Output\n\n")
	for _, t := range tasks {
		if t.Status != models.StatusCompleted {
			continue
		}
		b.WriteString("## [" + t.Domain + "] " + t.Description + "\n")
		b.WriteString(t.Output + "\n\n")
	}
	return b.String()
}

// TotalCost sums cost across the whole DAG for the Cost Tracker widget.
func TotalCost(tasks []models.Task) (spent float64, savedVsTopTier float64) {
	for _, t := range tasks {
		spent += t.CostUSD
		// "saved" = what it would have cost had every task run gpt-4o from the start.
		topTierCost := estimateCost("gpt-4o", t.TokensUsed)
		if topTierCost > t.CostUSD {
			savedVsTopTier += topTierCost - t.CostUSD
		}
	}
	return
}
