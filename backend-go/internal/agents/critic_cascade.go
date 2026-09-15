// backend-go/internal/agents/critic_cascade.go
package agents

import (
	"context"
	"strings"
)

// CriticEvaluate performs a lightweight quality check on a completed node's result.
// For coding nodes, the real grading happens inside RunCodingTask via critic.Analyze.
// This function acts as a final gate for non-coding results.
func CriticEvaluate(ctx context.Context, result string) bool {
	return strings.TrimSpace(result) != ""
}