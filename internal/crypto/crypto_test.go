// Copyright 2024 Qubership
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crypto

import (
	"crypto/rand"
	"testing"
)

func TestNewCrypto(t *testing.T) {
	// Test with valid 32-byte key (AES-256)
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	crypto, err := NewCrypto(key)
	if err != nil {
		t.Errorf("Expected no error for valid key, got: %v", err)
	}
	if crypto == nil {
		t.Error("Expected crypto instance to be created")
	}
}

func TestNewCrypto_InvalidKey(t *testing.T) {
	// Test with invalid key size
	key := make([]byte, 10) // Invalid size for AES

	_, err := NewCrypto(key)
	if err == nil {
		t.Error("Expected error for invalid key size")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	crypto, err := NewCrypto(key)
	if err != nil {
		t.Fatalf("Failed to create crypto: %v", err)
	}

	originalText := "Hello, World! This is a test message."

	// Encrypt
	encrypted, err := crypto.Encrypt([]byte(originalText))
	if err != nil {
		t.Errorf("Expected no error during encryption, got: %v", err)
	}
	if encrypted == "" {
		t.Error("Expected encrypted text to be non-empty")
	}

	// Decrypt
	decrypted, err := crypto.Decrypt([]byte(encrypted))
	if err != nil {
		t.Errorf("Expected no error during decryption, got: %v", err)
	}
	if decrypted != originalText {
		t.Errorf("Expected decrypted text to match original, got: %s", decrypted)
	}
}

func TestDecrypt_InvalidData(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	crypto, err := NewCrypto(key)
	if err != nil {
		t.Fatalf("Failed to create crypto: %v", err)
	}

	// Test with data too short for nonce
	_, err = crypto.Decrypt([]byte("short"))
	if err == nil {
		t.Error("Expected error for data too short")
	}

	// Test with invalid encrypted data
	_, err = crypto.Decrypt([]byte("invalidencrypteddata"))
	if err == nil {
		t.Error("Expected error for invalid encrypted data")
	}
}

func TestEncryptDecrypt_EmptyText(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	crypto, err := NewCrypto(key)
	if err != nil {
		t.Fatalf("Failed to create crypto: %v", err)
	}

	originalText := ""

	// Encrypt
	encrypted, err := crypto.Encrypt([]byte(originalText))
	if err != nil {
		t.Errorf("Expected no error during encryption, got: %v", err)
	}

	// Decrypt
	decrypted, err := crypto.Decrypt([]byte(encrypted))
	if err != nil {
		t.Errorf("Expected no error during decryption, got: %v", err)
	}
	if decrypted != originalText {
		t.Errorf("Expected decrypted text to match original, got: %s", decrypted)
	}
}
