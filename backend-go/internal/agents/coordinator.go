package agents

import (
	"sync"

	"llm-orchestrator/backend-go/internal/models"
)

// BroadcastFn lets the coordinator push live status to WebSocket clients
// without importing the ws package directly (keeps agents decoupled/testable).
type BroadcastFn func(eventType string, data interface{})

// RunPipeline executes the full Coordinator-Worker flow for one prompt:
//   Planner (ToT)  ->  parallel Workers per DAG level  ->  Critic cascade  ->  Synthesizer
func RunPipeline(req models.PlanRequest, broadcast BroadcastFn) models.DAGPlan {
	plan := PlanToT(req.Prompt)
	broadcast("plan_ready", plan)

	tasksByID := map[string]*models.Task{}
	for i := range plan.Tasks {
		tasksByID[plan.Tasks[i].ID] = &plan.Tasks[i]
	}

	done := map[string]bool{}
	queueDepth := len(plan.Tasks)

	for len(done) < len(plan.Tasks) {
		var wave []*models.Task
		for id, t := range tasksByID {
			if done[id] {
				continue
			}
			ready := true
			for _, dep := range t.DependsOn {
				if !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, t)
			}
		}
		if len(wave) == 0 {
			break // safety: dependency cycle, bail out
		}

		var wg sync.WaitGroup
		for _, t := range wave {
			wg.Add(1)
			go func(task *models.Task) {
				defer wg.Done()
				runTaskWithCascade(task, queueDepth, req.SLA.QualityWeight, req.Keys, broadcast)
			}(t)
		}
		wg.Wait()

		for _, t := range wave {
			done[t.ID] = true
			queueDepth--
		}
	}

	// write back mutated tasks into plan.Tasks slice order
	for i := range plan.Tasks {
		plan.Tasks[i] = *tasksByID[plan.Tasks[i].ID]
	}
	return plan
}

func runTaskWithCascade(t *models.Task, queueDepth int, qualityBias float64, keys models.APIKeyBundle, broadcast BroadcastFn) {
	for {
		ExecuteTask(t, queueDepth, qualityBias, keys)
		broadcast("task_update", *t)

		verdict := Review(*t)
		if verdict.Passed {
			return
		}
		if !verdict.Escalate || !Escalate(t) {
			// nothing left to escalate to; accept best-effort result
			return
		}
		broadcast("task_update", *t) // status now "escalated"
	}
}
