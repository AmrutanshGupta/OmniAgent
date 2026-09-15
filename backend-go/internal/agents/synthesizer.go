// backend-go/internal/agents/synthesizer.go
package agents

import (
	"fmt"
	"strings"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
)

func getTopologicalOrder(dag *models.DAG) []string {
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for id := range dag.Nodes {
		inDegree[id] = 0
	}
	for _, edge := range dag.Edges {
		from, to := edge[0], edge[1]
		adj[from] = append(adj[from], to)
		inDegree[to]++
	}
	var q []string
	for id, deg := range inDegree {
		if deg == 0 {
			q = append(q, id)
		}
	}
	var order []string
	for len(q) > 0 {
		curr := q[0]
		q = q[1:]
		order = append(order, curr)
		for _, next := range adj[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				q = append(q, next)
			}
		}
	}
	return order
}

func Synthesize(dag *models.DAG) string {
	var b strings.Builder
	b.WriteString("# Final Synthesized Output\n\n")

	order := getTopologicalOrder(dag)

	for _, id := range order {
		t := dag.Nodes[id]
		if t == nil || t.Status != models.StateSucceeded {
			continue
		}

		domainStr := "General"
		if t.Domain == 1 {
			domainStr = "Code"
		}

		b.WriteString(fmt.Sprintf("## [%s] Node: %s\n", domainStr, id))
		b.WriteString(fmt.Sprintf("Task: %s\n", t.Task))
		b.WriteString(t.Result)
		b.WriteString("\n\n")
	}

	return b.String()
}

func TotalCost(dag *models.DAG) (spent float64, savedVsTopTier float64) {
	for _, t := range dag.Nodes {
		spent += t.CostUSD

		topTierCost := estimateCost("gpt-4o", t.TokensUsed)
		if topTierCost > t.CostUSD {
			savedVsTopTier += topTierCost - t.CostUSD
		}
	}
	return spent, savedVsTopTier
}

func estimateCost(model string, tokens int) float64 {
	if model == "gpt-4o" {
		return float64(tokens) * 0.000015
	}
	return 0.0
}