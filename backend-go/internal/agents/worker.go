package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/config"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/security"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
)

type RouteRequest struct {
	WQ float64 `json:"wq"`
	WC float64 `json:"wc"`
	WL float64 `json:"wl"`
}

type RouteResponse struct {
	OptimalModel    string  `json:"optimal_model"`
	ExpectedLatency float64 `json:"expected_latency"`
	ExpectedCost    float64 `json:"expected_cost"`
}

// AssignTier returns a sensible tier fallback.
// The actual routing is performed by router_client.GetOptimalRoute in coordinator.go.
// This function is retained for callers that need a synchronous tier before the
// registry cache is populated.
func AssignTier(domain int, mlTier int, keys map[string]string, cfg *config.Config) string {
	// Map ML tier index to tier string
	switch mlTier {
	case 0:
		if keys["openai"] != "" {
			return "gpt-4o"
		}
	case 1:
		if keys["anthropic"] != "" {
			return "sonnet"
		}
	case 2:
		if keys["google"] != "" {
			return "flash"
		}
	}
	// Free-tier fallbacks
	if keys["groq"] != "" {
		return "groq"
	}
	return "hf"
}

type ExecutorRequest struct {
	SessionID      string            `json:"session_id"`
	NodeID         string            `json:"node_id"`
	Task           string            `json:"task"`
	OriginalPrompt string            `json:"original_prompt"`
	Tier           string            `json:"tier"`
	Keys           map[string]string `json:"keys"`
}

type ExecutorResponse struct {
	Result     string `json:"result"`
	TokensUsed int    `json:"tokens_used"`
}

func ExecuteReActLoop(ctx context.Context, node *models.TaskNode, tierStr string, originalPrompt string, cfg *config.Config) error {
	node.ThoughtTrace = append(node.ThoughtTrace, fmt.Sprintf("Routing to model tier: %s", tierStr))

	provider := "openai" // default
	switch tierStr {
	case "sonnet":
		provider = "anthropic"
	case "flash":
		provider = "google"
	case "groq":
		provider = "groq"
	case "hf":
		provider = "huggingface"
	}

	credService := security.NewCredentialService(cfg.EncryptionKey)
	decryptedKey, err := credService.GetDecryptedKey(ctx, provider)
	if err != nil && provider != "hf" && provider != "groq" {
		// Log error but we might proceed if it's a free tier
		fmt.Printf("Warning: failed to decrypt key for provider %s: %v\n", provider, err)
	}

	decryptedKeys := map[string]string{
		provider: decryptedKey,
	}
	
	defer func() {
		// strict memclr is harder in Go with strings, but we can clear the map
		decryptedKeys[provider] = ""
	}()

	reqPayload := ExecutorRequest{
		SessionID:      node.SessionID,
		NodeID:         node.ID,
		Task:           node.Task,
		OriginalPrompt: originalPrompt,
		Tier:           tierStr,
		Keys:           decryptedKeys,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		node.Status = models.StateFailed
		return fmt.Errorf("failed to marshal executor request: %v", err)
	}

	pythonURL := cfg.PythonRouterURL
	if pythonURL == "" {
		pythonURL = "http://backend-python:8000"
	}
	url := fmt.Sprintf("%s/v1/execute-node", pythonURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		node.Status = models.StateFailed
		return fmt.Errorf("failed to create executor request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Timeout should be long enough for the LangChain agent to complete
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		node.Status = models.StateFailed
		return fmt.Errorf("executor API error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		node.Status = models.StateFailed
		return fmt.Errorf("executor API returned status: %d", resp.StatusCode)
	}

	var execResp ExecutorResponse
	if err := json.NewDecoder(resp.Body).Decode(&execResp); err != nil {
		node.Status = models.StateFailed
		return fmt.Errorf("failed to decode executor response: %v", err)
	}

	node.Result = execResp.Result
	node.TokensUsed = execResp.TokensUsed
	node.Status = models.StateSucceeded

	// Cost calculation based on heuristic or exact if returned
	switch tierStr {
	case "gpt-4o":
		node.CostUSD = float64(node.TokensUsed) * 0.000005
	case "sonnet":
		node.CostUSD = float64(node.TokensUsed) * 0.000003
	case "flash":
		node.CostUSD = float64(node.TokensUsed) * 0.00000015
	case "groq", "hf":
		node.CostUSD = 0.0
	}

	return nil
}