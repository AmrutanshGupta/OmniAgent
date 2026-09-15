package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/security"
)

type SubTask struct {
	ID        string   `json:"id"`
	Task      string   `json:"task"`
	Domain    int      `json:"domain"`
	DependsOn []string `json:"depends_on"`
}

type PlannerRequest struct {
	Prompt string            `json:"prompt"`
	Keys   map[string]string `json:"keys"`
}

func GenerateDAG(ctx context.Context, prompt string, cfg *config.Config) (*models.DAG, error) {
	credService := security.NewCredentialService(cfg.EncryptionKey)
	
	decryptedKeys := make(map[string]string)
	if k, err := credService.GetDecryptedKey(ctx, "openai"); err == nil { decryptedKeys["openai"] = k }
	if k, err := credService.GetDecryptedKey(ctx, "anthropic"); err == nil { decryptedKeys["anthropic"] = k }
	if k, err := credService.GetDecryptedKey(ctx, "google"); err == nil { decryptedKeys["google"] = k }
	if k, err := credService.GetDecryptedKey(ctx, "groq"); err == nil { decryptedKeys["groq"] = k }
	
	defer func() {
		for k := range decryptedKeys {
			decryptedKeys[k] = ""
		}
	}()

	userID := ctx.Value(models.UserIDKey).(string)
	sessionID := ctx.Value(models.SessionIDKey).(string)

	reqPayload := PlannerRequest{
		Prompt: prompt,
		Keys:   decryptedKeys,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal planner request: %v", err)
	}

	pythonURL := cfg.PythonRouterURL
	if pythonURL == "" {
		pythonURL = "http://backend-python:8000"
	}
	url := fmt.Sprintf("%s/v1/plan", pythonURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("planner request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("planner API returned status: %d", resp.StatusCode)
	}

	var parsedTasks []SubTask
	if err := json.NewDecoder(resp.Body).Decode(&parsedTasks); err != nil {
		return nil, fmt.Errorf("failed to decode planner response: %v", err)
	}

	dag := &models.DAG{
		SessionID: sessionID, UserID: userID, Status: "pending_approval",
		Nodes: make(map[string]*models.TaskNode), Edges: [][]string{},
	}

	for _, st := range parsedTasks {
		// Tier logic is now handled by Python's ELO router, but we set a default here.
		// The actual routing happens in execution or we could call Python router here.
		// For now, setting to "gpt-4o" as placeholder, it will be overridden or routed in worker.
		node := &models.TaskNode{
			ID: st.ID, Task: st.Task, Status: models.StatePending, CreatedAt: time.Now(),
			Domain: st.Domain, PlannedTier: "gpt-4o", UserID: userID, SessionID: sessionID,
		}

		for _, depID := range st.DependsOn {
			dag.Edges = append(dag.Edges, []string{depID, st.ID})
			node.ParentID = depID
		}

		dag.Nodes[st.ID] = node
	}

	return dag, nil
}