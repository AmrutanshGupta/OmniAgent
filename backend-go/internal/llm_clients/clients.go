package llm_clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/rs/zerolog/log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/registry"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/security"
	"go.opentelemetry.io/otel"
)

var cache *registry.Cache
var credService *security.CredentialService

func InitResolver(ctx context.Context, c *registry.Cache, cs *security.CredentialService) {
	cache = c
	credService = cs
}

type tierModel struct{ Provider, Model string }

var emergencyDefaults = map[string]tierModel{
	"flash":  {"google", "gemini-1.5-flash"}, 
	"sonnet": {"anthropic", "claude-3-5-sonnet-20240620"},
	"gpt-4o": {"openai", "gpt-4o"},
	"groq":   {"groq", "openai/gpt-oss-120b"}, 
	"hf":     {"huggingface", "HuggingFaceH4/zephyr-7b-beta"}, 
}

func mapOldTierToNewTier(tier string) registry.Tier {
	switch tier {
	case "flash":
		return registry.TierFast
	case "sonnet":
		return registry.TierReasoning
	case "gpt-4o":
		return registry.TierFrontier
	case "groq":
		return registry.TierEconomy
	default:
		return registry.TierEconomy
	}
}

func resolveTier(tier string) (tierModel, bool) {
	if tier == "hf" {
		return emergencyDefaults["hf"], true
	}
	if cache != nil {
		models := cache.GetModelsByTier(mapOldTierToNewTier(tier))
		if len(models) > 0 {
			return tierModel{models[0].Provider, models[0].ModelID}, true
		}
	}
	tm, ok := emergencyDefaults[tier]
	return tm, ok
}

type CompletionResult struct {
	Text       string
	TokensUsed int
	Simulated  bool
}

var httpClient = &http.Client{Timeout: 45 * time.Second}

func Complete(ctx context.Context, tier, systemPrompt, userPrompt string) (CompletionResult, error) {
	tracer := otel.Tracer("llm_client")
	ctx, span := tracer.Start(ctx, "LLM_Complete_"+tier)
	defer span.End()

	tm, ok := resolveTier(tier)
	if !ok {
		tm, _ = resolveTier("hf")
	}

	maxRetries := 5
	baseDelay := 2 * time.Second

	var res CompletionResult
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = nil // reset error on new attempt
		
		var key string
		if credService != nil {
			key, err = credService.GetDecryptedKey(ctx, tm.Provider)
		}
		keyBytes := []byte(key)
		
		if err == nil {
			switch tm.Provider {
			case "anthropic":
				if len(keyBytes) > 0 { res, err = executeWithBreaker("anthropic", func() (CompletionResult, error) { return callAnthropic(keyBytes, tm.Model, systemPrompt, userPrompt) }) } else { err = fmt.Errorf("no key") }
			case "openai":
				if len(keyBytes) > 0 { res, err = executeWithBreaker("openai", func() (CompletionResult, error) { return callOpenAI(keyBytes, tm.Model, systemPrompt, userPrompt) }) } else { err = fmt.Errorf("no key") }
			case "google":
				if len(keyBytes) > 0 { res, err = executeWithBreaker("google", func() (CompletionResult, error) { return callGoogle(keyBytes, tm.Model, systemPrompt, userPrompt) }) } else { err = fmt.Errorf("no key") }
			case "groq":
				if len(keyBytes) > 0 { res, err = executeWithBreaker("groq", func() (CompletionResult, error) { return callGroq(keyBytes, tm.Model, systemPrompt, userPrompt) }) } else { err = fmt.Errorf("no key") }
			case "huggingface":
				if len(keyBytes) > 0 { res, err = executeWithBreaker("huggingface", func() (CompletionResult, error) { return callHuggingFace(keyBytes, tm.Model, systemPrompt, userPrompt) }) } else { err = fmt.Errorf("no key") }
			default:
				return simulate(tier, userPrompt), nil
			}
		}

		if err != nil && (err.Error() == "no key" || strings.Contains(err.Error(), "no encrypted key found")) {
			return simulate(tier, userPrompt), nil
		}

		if err == nil {
			return res, nil
		}

		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate_limit_exceeded") {
			if attempt < maxRetries {
				delay := baseDelay

				// Groq provides exact wait times: "Please try again in 27.5s."
				re := regexp.MustCompile(`Please try again in ([0-9.]+)s`)
				if match := re.FindStringSubmatch(err.Error()); len(match) > 1 {
					if parsed, parseErr := strconv.ParseFloat(match[1], 64); parseErr == nil {
						delay = time.Duration(parsed*1000)*time.Millisecond + time.Second // +1s buffer
					}
				}

				log.Warn().Str("provider", tm.Provider).Dur("delay", delay).Int("attempt", attempt).Int("max", maxRetries).Msg("[llm_clients] Rate limit hit, retrying")
				time.Sleep(delay)
				baseDelay *= 2 // Exponential backoff
				continue
			}
		}

		return res, err
	}

	return res, fmt.Errorf("max retries exceeded: %v", err)
}

func simulate(tier, userPrompt string) CompletionResult {
	return CompletionResult{
		Text:       fmt.Sprintf("[SIMULATED %s output — add a BYOK key in Settings for a real completion]\nTask: %s\nResult: OK", tier, truncate(userPrompt, 120)),
		TokensUsed: 180,
		Simulated:  true,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}

// --- UPDATED API CALLS (Now they return real errors on Rate Limits!) ---

func callAnthropic(key []byte, model, system, user string) (CompletionResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model, "max_tokens": 1000, "system": system,
		"messages": []map[string]string{{"role": "user", "content": user}},
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", string(key))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil { return CompletionResult{}, fmt.Errorf("network error: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResult{}, fmt.Errorf("Anthropic API Error %d: %s", resp.StatusCode, string(data))
	}

	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Content []struct { Text string `json:"text"` } `json:"content"`
		Usage struct { OutputTokens int `json:"output_tokens"` } `json:"usage"`
	}
	json.Unmarshal(data, &parsed)
	return CompletionResult{Text: parsed.Content[0].Text, TokensUsed: parsed.Usage.OutputTokens}, nil
}

func callOpenAI(key []byte, model, system, user string) (CompletionResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
	})
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil { return CompletionResult{}, fmt.Errorf("network error: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResult{}, fmt.Errorf("OpenAI API Error %d: %s", resp.StatusCode, string(data))
	}

	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Choices []struct { Message struct { Content string `json:"content"` } `json:"message"` } `json:"choices"`
		Usage struct { CompletionTokens int `json:"completion_tokens"` } `json:"usage"`
	}
	json.Unmarshal(data, &parsed)
	return CompletionResult{Text: parsed.Choices[0].Message.Content, TokensUsed: parsed.Usage.CompletionTokens}, nil
}

func callGoogle(key []byte, model, system, user string) (CompletionResult, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, string(key))
	body, _ := json.Marshal(map[string]interface{}{
		"systemInstruction": map[string]interface{}{"parts": []map[string]string{{"text": system}}},
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": user}}}},
	})
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil { return CompletionResult{}, fmt.Errorf("network error: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResult{}, fmt.Errorf("Google API Error %d: %s", resp.StatusCode, string(data))
	}

	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Candidates []struct { Content struct { Parts []struct { Text string `json:"text"` } `json:"parts"` } `json:"content"` } `json:"candidates"`
	}
	json.Unmarshal(data, &parsed)
	return CompletionResult{Text: parsed.Candidates[0].Content.Parts[0].Text, TokensUsed: len(user) / 4}, nil
}

func callGroq(key []byte, model, system, user string) (CompletionResult, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
	})
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil { return CompletionResult{}, fmt.Errorf("network error: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResult{}, fmt.Errorf("Groq API Error %d: %s", resp.StatusCode, string(data))
	}

	data, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Choices []struct { Message struct { Content string `json:"content"` } `json:"message"` } `json:"choices"`
		Usage struct { CompletionTokens int `json:"completion_tokens"` } `json:"usage"`
	}
	json.Unmarshal(data, &parsed)
	return CompletionResult{Text: parsed.Choices[0].Message.Content, TokensUsed: parsed.Usage.CompletionTokens}, nil
}

func callHuggingFace(key []byte, model, system, user string) (CompletionResult, error) {
	url := "https://api-inference.huggingface.co/models/" + model
	prompt := fmt.Sprintf("<|begin_of_text|><|start_header_id|>system<|end_header_id|>\n\n%s<|eot_id|><|start_header_id|>user<|end_header_id|>\n\n%s<|eot_id|><|start_header_id|>assistant<|end_header_id|>\n\n", system, user)
	body, _ := json.Marshal(map[string]interface{}{
		"inputs": prompt, "parameters": map[string]interface{}{"max_new_tokens": 1000, "return_full_text": false},
	})
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil { return CompletionResult{}, fmt.Errorf("network error: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return CompletionResult{}, fmt.Errorf("HF API Error %d: %s", resp.StatusCode, string(data))
	}

	data, _ := io.ReadAll(resp.Body)
	var parsed []struct { GeneratedText string `json:"generated_text"` }
	json.Unmarshal(data, &parsed)
	
	if len(parsed) == 0 { return CompletionResult{}, fmt.Errorf("HF returned empty output") }
	return CompletionResult{Text: parsed[0].GeneratedText, TokensUsed: len(parsed[0].GeneratedText) / 4}, nil
}