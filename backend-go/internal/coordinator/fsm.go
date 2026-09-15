package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Engine struct {
	dagMu sync.Mutex
	dag   *models.DAG
	db    *mongo.Database
}

func NewEngine(dag *models.DAG, db *mongo.Database) *Engine {
	return &Engine{
		dag: dag,
		db:  db,
	}
}

// PersistDAG inserts the initial DAG nodes into MongoDB.
func (e *Engine) PersistDAG(ctx context.Context) error {
	if e.db == nil {
		return nil
	}
	var docs []interface{}
	for _, n := range e.dag.Nodes {
		n.Version = 1
		docs = append(docs, n)
	}
	if len(docs) > 0 {
		_, err := e.db.Collection("dag_nodes").InsertMany(ctx, docs)
		return err
	}
	return nil
}

var validTransitions = map[models.NodeState][]models.NodeState{
	models.StatePending:    {models.StateReady, models.StateCancelled, models.StateFailed},
	models.StateReady:      {models.StateRunning, models.StateCancelled},
	models.StateRunning:    {models.StateEvaluating, models.StateFailed, models.StateCancelled, models.StateSucceeded, models.StateReady}, // Ready added for sweeper reclamation
	models.StateEvaluating: {models.StateSucceeded, models.StateRetrying, models.StateEscalated, models.StateFailed},
	models.StateRetrying:   {models.StateRunning, models.StateCancelled},
	models.StateEscalated:  {models.StateRunning, models.StateCancelled},
	models.StateSucceeded:  {},
	models.StateFailed:     {},
	models.StateCancelled:  {},
}

func isValidTransition(from, to models.NodeState) bool {
	if from == to {
		return true
	}
	for _, a := range validTransitions[from] {
		if a == to {
			return true
		}
	}
	return false
}

// TransitionNode atomically advances a node's state using Optimistic Concurrency and Outbox pattern.
func (e *Engine) TransitionNode(ctx context.Context, nodeID string, expected, next models.NodeState, payload interface{}) error {
	e.dagMu.Lock()
	defer e.dagMu.Unlock()

	node, ok := e.dag.Nodes[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if node.Status != expected {
		return fmt.Errorf("CAS failed for node %s: expected %s, got %s", nodeID, expected, node.Status)
	}

	if !isValidTransition(node.Status, next) {
		return fmt.Errorf("invalid FSM transition from %s to %s for node %s", node.Status, next, nodeID)
	}

	// 1. Prepare updates
	newVersion := node.Version + 1
	var leaseExpiresAt time.Time
	
	// Atomic lease claiming
	if next == models.StateRunning {
		leaseExpiresAt = time.Now().Add(5 * time.Minute)
	}

	// 2. Perform DB update if available
	if e.db != nil {
		event := models.TaskEvent{
			SessionID: e.dag.SessionID,
			Type:      models.EventNodeTransition,
			Payload: map[string]interface{}{
				"node_id": nodeID,
				"from":    expected,
				"to":      next,
				"details": payload,
			},
			Timestamp: time.Now(),
		}

		// Use a transaction to bind state mutations and event emissions
		session, err := e.db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)

		callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
			// Update Node
			update := bson.M{
				"$set": bson.M{
					"status": next,
					"version": newVersion,
				},
			}
			if !leaseExpiresAt.IsZero() {
				update["$set"].(bson.M)["lease_expires_at"] = leaseExpiresAt
			} else {
				// Clear lease if not running
				update["$unset"] = bson.M{"lease_expires_at": ""}
			}

			res, err := e.db.Collection("dag_nodes").UpdateOne(sessCtx, 
				bson.M{"id": nodeID, "version": node.Version},
				update,
			)
			if err != nil {
				return nil, err
			}
			if res.ModifiedCount == 0 {
				return nil, fmt.Errorf("concurrent modification detected for node %s", nodeID)
			}

			// Insert Outbox Event
			_, err = e.db.Collection("outbox_events").InsertOne(sessCtx, event)
			if err != nil {
				return nil, err
			}
			
			return nil, nil
		}

		_, err = session.WithTransaction(ctx, callback)
		if err != nil {
			// Concurrent claims return graceful misses to caller
			return fmt.Errorf("transaction failed: %w", err)
		}
	}

	// 3. Update in-memory state
	node.Status = next
	node.Version = newVersion
	node.LeaseExpiresAt = leaseExpiresAt

	return nil
}

func (e *Engine) GetDAG() *models.DAG {
	e.dagMu.Lock()
	defer e.dagMu.Unlock()
	return e.dag
}

func (e *Engine) UpdateResult(nodeID, result string, tokensUsed int, costUSD float64) {
	e.dagMu.Lock()
	defer e.dagMu.Unlock()
	if node, ok := e.dag.Nodes[nodeID]; ok {
		node.Result = result
		node.TokensUsed = tokensUsed
		node.CostUSD = costUSD
	}
}

func (e *Engine) UpdateTier(nodeID, tier string) {
	e.dagMu.Lock()
	defer e.dagMu.Unlock()
	if node, ok := e.dag.Nodes[nodeID]; ok {
		node.ActualTier = tier
	}
}

// StartSweeper runs a background task to reclaim stranded nodes.
func (e *Engine) StartSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.sweepStrandedTasks(ctx)
			}
		}
	}()
}

func (e *Engine) sweepStrandedTasks(ctx context.Context) {
	e.dagMu.Lock()
	var stranded []string
	now := time.Now()
	for id, node := range e.dag.Nodes {
		if node.Status == models.StateRunning && !node.LeaseExpiresAt.IsZero() && now.After(node.LeaseExpiresAt) {
			stranded = append(stranded, id)
		}
	}
	e.dagMu.Unlock()

	for _, id := range stranded {
		// Attempt to reclaim. Expected state is RUNNING, next state is READY for retry.
		_ = e.TransitionNode(ctx, id, models.StateRunning, models.StateReady, "Reclaimed by heartbeat sweeper")
	}
}
