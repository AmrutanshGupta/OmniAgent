package agents

import (
	"strings"

	"llm-orchestrator/backend-go/internal/models"
)

// CriticVerdict describes whether a task's output passed QA.
type CriticVerdict struct {
	Passed  bool
	Reason  string
	Escalate bool
}

// Review implements the FrugalGPT cascade: cheap tier first, and only pay
// for a smarter model if the cheap one's output looks broken. Heuristics
// stand in for a real LLM-as-judge call (kept dependency-free & fast);
// swap in an actual Critic LLM call here for production use.
func Review(t models.Task) CriticVerdict {
	out := strings.TrimSpace(t.Output)

	if out == "" {
		return CriticVerdict{Passed: false, Reason: "empty output", Escalate: true}
	}
	if strings.Contains(strings.ToLower(out), "error:") {
		return CriticVerdict{Passed: false, Reason: "worker reported an error", Escalate: true}
	}
	if t.Domain == "coding" && looksLikeBrokenCode(out) {
		return CriticVerdict{Passed: false, Reason: "syntax/structure looks incomplete", Escalate: true}
	}
	if len(out) < 15 {
		return CriticVerdict{Passed: false, Reason: "suspiciously short output", Escalate: true}
	}
	return CriticVerdict{Passed: true, Reason: "looks good"}
}

func looksLikeBrokenCode(out string) bool {
	openBraces := strings.Count(out, "{")
	closeBraces := strings.Count(out, "}")
	openParen := strings.Count(out, "(")
	closeParen := strings.Count(out, ")")
	return openBraces != closeBraces || openParen != closeParen
}

// Escalate bumps a task to the next rung of its FrugalGPT cascade ladder.
// Returns false if already at the top (nothing left to escalate to).
func Escalate(t *models.Task) bool {
	if t.CascadeIdx+1 >= len(t.Cascade) {
		return false
	}
	t.CascadeIdx++
	t.Status = models.StatusEscalated
	return true
}
