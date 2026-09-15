package llm_clients

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sony/gobreaker"
)

func TestCircuitBreaker_TripsOnNetworkErrors(t *testing.T) {
	provider := "test-provider-trip"
	
	actionNetworkErr := func() (CompletionResult, error) {
		return CompletionResult{}, fmt.Errorf("network error: connection refused")
	}

	actionSuccess := func() (CompletionResult, error) {
		return CompletionResult{Text: "Success"}, nil
	}

	// 1 fail
	_, err := executeWithBreaker(provider, actionNetworkErr)
	if err == nil { t.Errorf("expected error") }
	
	// 2 fail
	_, err = executeWithBreaker(provider, actionNetworkErr)
	if err == nil { t.Errorf("expected error") }

	// 3 fail - trips
	_, err = executeWithBreaker(provider, actionNetworkErr)
	if err == nil { t.Errorf("expected error") }

	// 4 attempt - should immediately fail with ErrProviderUnavailable
	_, err = executeWithBreaker(provider, actionSuccess)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("expected ErrProviderUnavailable, got %v", err)
	}
	
	cb := getBreaker(provider)
	if cb.State() != gobreaker.StateOpen {
		t.Errorf("expected breaker to be OPEN")
	}
}

func TestCircuitBreaker_Ignores4xxErrors(t *testing.T) {
	provider := "test-provider-ignore"
	
	action4xxErr := func() (CompletionResult, error) {
		return CompletionResult{}, fmt.Errorf("API Error 429: Too Many Requests")
	}

	// 5 fails of 4xx
	for i := 0; i < 5; i++ {
		_, err := executeWithBreaker(provider, action4xxErr)
		if err == nil { t.Errorf("expected error") }
	}
	
	cb := getBreaker(provider)
	if cb.State() == gobreaker.StateOpen {
		t.Errorf("expected breaker to be CLOSED since 4xx should not trip it")
	}
}
