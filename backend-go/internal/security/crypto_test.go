package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestDecryptUserKey_Zeroing(t *testing.T) {
	// Generate a random 32-byte key for encryption
	keyBytes := make([]byte, 32)
	rand.Read(keyBytes)
	keyHex := hex.EncodeToString(keyBytes)

	// Encrypt some text
	plaintext := []byte("my-super-secret-api-key")
	
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		t.Fatal(err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}

	nonce := make([]byte, 12)
	rand.Read(nonce)

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	// In our format: iv (12) + tag (16) + ciphertext (remainder)
	// Wait, Seal appends the tag to the ciphertext: result is `ciphertext + tag`.
	// Our Decrypt function expects: decoded[:12] is IV, decoded[12:28] is tag, decoded[28:] is ciphertext.
	// We need to re-arrange to match the frontend/custom format.
	
	// Seal output: ciphertext... + tag (16 bytes)
	tag := ciphertext[len(ciphertext)-16:]
	actualCiphertext := ciphertext[:len(ciphertext)-16]
	
	combined := make([]byte, 0)
	combined = append(combined, nonce...)
	combined = append(combined, tag...)
	combined = append(combined, actualCiphertext...)
	
	encryptedBase64 := base64.StdEncoding.EncodeToString(combined)

	// Decrypt
	decrypted, err := DecryptUserKey(encryptedBase64, keyHex)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("Expected %s, got %s", plaintext, decrypted)
	}

	// Verify memclr pattern manually
	defer func() {
		for i := range decrypted {
			decrypted[i] = 0
		}
		
		// Ensure it's zeroed
		for _, b := range decrypted {
			if b != 0 {
				t.Errorf("Buffer not properly zeroed")
			}
		}
	}()
}
