package security

import (
	"context"
	"errors"
	"fmt"
	"github.com/AmrutanshGupta/OmniAgent/backend-go/internal/models"
)

type CredentialService struct {
	encryptionKey string
}

func NewCredentialService(key string) *CredentialService {
	return &CredentialService{
		encryptionKey: key,
	}
}

// GetDecryptedKey fetches the encrypted key from the context, decrypts it, and returns the raw key.
// It fails fast if the context is missing the keys map, the provider is missing, or decryption fails.
func (s *CredentialService) GetDecryptedKey(ctx context.Context, provider string) (string, error) {
	keys, ok := ctx.Value(models.APIKeysKey).(map[string]string)
	if !ok {
		return "", errors.New("API keys map not found in context")
	}

	encryptedBase64, exists := keys[provider]
	if !exists || encryptedBase64 == "" {
		return "", fmt.Errorf("no encrypted key found for provider: %s", provider)
	}

	// Wait, DecryptUserKey in crypto.go uses the globally initialized encryptionKey.
	// We need to modify DecryptUserKey to accept the key or initialize it.
	// For now we'll call DecryptUserKey, assuming it's initialized correctly in main or we'll refactor crypto.go next.
	
	decryptedBytes, err := DecryptUserKey(encryptedBase64, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key for %s: %w", provider, err)
	}

	return string(decryptedBytes), nil
}
