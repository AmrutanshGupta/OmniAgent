package coordinator

import (
	"context"
	"sync"
	"testing"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
)

func TestFSM_Race(t *testing.T) {
	dag := &models.DAG{
		SessionID: "test-session",
		Nodes: map[string]*models.TaskNode{
			"node1": {ID: "node1", Status: models.StatePending},
		},
	}

	engine := NewEngine(dag, nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	// Simulate multiple concurrent transition attempts
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Many will fail the expected state check, which is fine and thread-safe
			_ = engine.TransitionNode(ctx, "node1", models.StatePending, models.StateReady, nil)
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// This might succeed if one of the StateReady transitions succeeded
			_ = engine.TransitionNode(ctx, "node1", models.StateReady, models.StateRunning, nil)
		}(i)
	}

	wg.Wait()

	finalState := engine.GetDAG().Nodes["node1"].Status
	if finalState != models.StatePending && finalState != models.StateReady && finalState != models.StateRunning {
		t.Fatalf("Unexpected final state: %s", finalState)
	}
}

func TestFSM_Transitions(t *testing.T) {
	dag := &models.DAG{
		SessionID: "test-session",
		Nodes: map[string]*models.TaskNode{
			"node1": {ID: "node1", Status: models.StatePending},
		},
	}
	engine := NewEngine(dag, nil)
	ctx := context.Background()

	err := engine.TransitionNode(ctx, "node1", models.StatePending, models.StateReady, nil)
	if err != nil {
		t.Errorf("Valid transition failed: %v", err)
	}

	err = engine.TransitionNode(ctx, "node1", models.StateReady, models.StateSucceeded, nil)
	if err == nil {
		t.Errorf("Invalid transition should have failed")
	}

	// Idempotent
	err = engine.TransitionNode(ctx, "node1", models.StateReady, models.StateReady, nil)
	if err != nil {
		t.Errorf("Idempotent transition failed: %v", err)
	}
}
