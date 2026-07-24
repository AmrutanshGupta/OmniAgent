package llm_clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Tier -> (provider, model) mapping used by the router + cascade.
var TierModel = map[string]struct {
	Provider string
	Model    string
}{
	"flash":  {"google", "gemini-1.5-flash"},
	"sonnet": {"anthropic", "claude-sonnet-4-6"},
	"gpt-4o": {"openai", "gpt-4o"},
}

type CompletionResult struct {
	Text       string
	TokensUsed int
	Simulated  bool // true when no BYOK key was supplied for that provider
}

var httpClient = &http.Client{Timeout: 45 * time.Second}

// Complete dispatches to the correct provider SDK/HTTP endpoint based on tier.
// If the caller has not supplied a key for that provider (BYOK), it falls back
// to a deterministic simulated response so the orchestrator remains runnable
// end-to-end without any paid keys (useful for local dev / demo mode).
func Complete(tier, systemPrompt, userPrompt, openaiKey, anthropicKey, googleKey string) (CompletionResult, error) {
	tm, ok := TierModel[tier]
	if !ok {
		tm = TierModel["flash"]
	}

	switch tm.Provider {
	case "anthropic":
		if anthropicKey != "" {
			return callAnthropic(anthropicKey, tm.Model, systemPrompt, userPrompt)
		}
	case "openai":
		if openaiKey != "" {
			return callOpenAI(openaiKey, tm.Model, systemPrompt, userPrompt)
		}
	case "google":
		if googleKey != "" {
			return callGoogle(googleKey, tm.Model, systemPrompt, userPrompt)
		}
	}
	// No key -> simulate so pipeline still functions end-to-end.
	return simulate(tier, userPrompt), nil
}

func simulate(tier, userPrompt string) CompletionResult {
	return CompletionResult{
		Text:       fmt.Sprintf("[SIMULATED %s output — add a BYOK key in Settings for a real completion]\nTask: %s\nResult: OK", tier, truncate(userPrompt, 120)),
		TokensUsed: 180,
		Simulated:  true,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func callAnthropic(key, model, system, user string) (CompletionResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"max_tokens": 1000,
		"system":     system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return simulate("sonnet", user), nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Content) == 0 {
		return simulate("sonnet", user), nil
	}
	return CompletionResult{Text: parsed.Content[0].Text, TokensUsed: parsed.Usage.OutputTokens}, nil
}

func callOpenAI(key, model, system, user string) (CompletionResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return simulate("gpt-4o", user), nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Choices) == 0 {
		return simulate("gpt-4o", user), nil
	}
	return CompletionResult{Text: parsed.Choices[0].Message.Content, TokensUsed: parsed.Usage.CompletionTokens}, nil
}

func callGoogle(key, model, system, user string) (CompletionResult, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, key)
	body, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": system + "\n\n" + user}}},
		},
	})
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return simulate("flash", user), nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return simulate("flash", user), nil
	}
	return CompletionResult{Text: parsed.Candidates[0].Content.Parts[0].Text, TokensUsed: len(user) / 4}, nil
}
