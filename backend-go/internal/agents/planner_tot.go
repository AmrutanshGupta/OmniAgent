package agents

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"llm-orchestrator/backend-go/internal/models"
)

// PlanToT implements a lightweight Tree-of-Thoughts planner:
//  1. Generate 3 candidate architectural decompositions of the prompt
//     ("sequential", "parallel-specialist", "hierarchical") concurrently.
//  2. Score each on scalability + cost.
//  3. Return the winning DAG of Task objects.
//
// This mirrors the PRD's "heavyweight model evaluates 3 approaches" step,
// but uses fast deterministic heuristics for the structural decomposition
// (domain classification + dependency shape) while the *content* of each
// task is still executed by a real LLM later in the Worker stage.
func PlanToT(prompt string) models.DAGPlan {
	candidates := []models.ToTCandidate{
		buildSequential(prompt),
		buildParallelSpecialist(prompt),
		buildHierarchical(prompt),
	}

	for i := range candidates {
		candidates[i].TotalScore = candidates[i].ScalabilityScr*0.5 + candidates[i].CostScore*0.5
	}

	winner := candidates[0]
	for _, c := range candidates[1:] {
		if c.TotalScore > winner.TotalScore {
			winner = c
		}
	}

	return models.DAGPlan{
		ID:     uuid.NewString(),
		Prompt: prompt,
		Winner: winner.Approach,
		Tasks:  winner.Tasks,
	}
}

func classifyDomains(prompt string) []string {
	p := strings.ToLower(prompt)
	domains := []string{}
	if strings.ContainsAny(p, "code") || strings.Contains(p, "implement") || strings.Contains(p, "function") || strings.Contains(p, "api") || strings.Contains(p, "bug") {
		domains = append(domains, "coding")
	}
	if strings.Contains(p, "data") || strings.Contains(p, "analy") || strings.Contains(p, "csv") || strings.Contains(p, "chart") {
		domains = append(domains, "data")
	}
	if strings.Contains(p, "write") || strings.Contains(p, "story") || strings.Contains(p, "creative") || strings.Contains(p, "marketing") || strings.Contains(p, "copy") {
		domains = append(domains, "creative")
	}
	if len(domains) == 0 {
		domains = []string{"coding", "data", "creative"}
	}
	return domains
}

func newTask(domain, desc string, deps []string) models.Task {
	return models.Task{
		ID:          uuid.NewString()[:8],
		Domain:      domain,
		Description: desc,
		DependsOn:   deps,
		Status:      models.StatusPending,
		Cascade:     []string{"flash", "sonnet", "gpt-4o"},
	}
}

func buildSequential(prompt string) models.ToTCandidate {
	domains := classifyDomains(prompt)
	var tasks []models.Task
	var prevID string
	for _, d := range domains {
		deps := []string{}
		if prevID != "" {
			deps = []string{prevID}
		}
		t := newTask(d, fmt.Sprintf("%s subtask for: %s", strings.Title(d), prompt), deps)
		tasks = append(tasks, t)
		prevID = t.ID
	}
	return models.ToTCandidate{
		Approach:       "sequential",
		ScalabilityScr: 0.4, // linear chain -> low parallelism
		CostScore:      0.8, // fewer concurrent model calls -> cheap
		Tasks:          tasks,
	}
}

func buildParallelSpecialist(prompt string) models.ToTCandidate {
	domains := classifyDomains(prompt)
	var tasks []models.Task
	for _, d := range domains {
		tasks = append(tasks, newTask(d, fmt.Sprintf("%s specialist handles: %s", strings.Title(d), prompt), []string{}))
	}
	// Add a final synthesis-dependent task if more than one domain
	if len(tasks) > 1 {
		var ids []string
		for _, t := range tasks {
			ids = append(ids, t.ID)
		}
		tasks = append(tasks, newTask("synthesis", "Merge specialist outputs", ids))
	}
	return models.ToTCandidate{
		Approach:       "parallel-specialist",
		ScalabilityScr: 0.9, // fully parallel workers
		CostScore:      0.6, // more concurrent calls -> pricier
		Tasks:          tasks,
	}
}

func buildHierarchical(prompt string) models.ToTCandidate {
	domains := classifyDomains(prompt)
	root := newTask("coding", "Decompose and coordinate: "+prompt, []string{})
	tasks := []models.Task{root}
	for _, d := range domains {
		tasks = append(tasks, newTask(d, fmt.Sprintf("%s branch under coordinator: %s", strings.Title(d), prompt), []string{root.ID}))
	}
	return models.ToTCandidate{
		Approach:       "hierarchical",
		ScalabilityScr: 0.7,
		CostScore:      0.7,
		Tasks:          tasks,
	}
}
