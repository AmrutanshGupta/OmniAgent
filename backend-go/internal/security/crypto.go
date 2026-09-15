// backend-go/internal/security/crypto.go
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

func DecryptUserKey(encryptedBase64, keyHex string) ([]byte, error) {
	if encryptedBase64 == "" {
		return nil, nil
	}

	if keyHex == "" {
		return nil, errors.New("ENCRYPTION_KEY is not set")
	}

	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, errors.New("ENCRYPTION_KEY is not a valid hex string")
	}
	// Zero out the decoded ENCRYPTION_KEY buffer when done just to be safe
	defer func() {
		for i := range keyBytes {
			keyBytes[i] = 0
		}
	}()

	if len(keyBytes) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must be exactly 32 bytes (64 hex characters)")
	}

	decoded, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return nil, err
	}

	if len(decoded) < 28 {
		return nil, errors.New("ciphertext too short")
	}

	iv := decoded[:12]
	tag := decoded[12:28]
	ciphertext := decoded[28:]


	goCiphertext := make([]byte, 0, len(ciphertext)+len(tag))
	goCiphertext = append(goCiphertext, ciphertext...)
	goCiphertext = append(goCiphertext, tag...)

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesGCM.Open(nil, iv, goCiphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}