package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEd25519KeyPair(t *testing.T) {
	privPEM, pubStr, err := GenerateEd25519KeyPair()
	assert.NoError(t, err)
	assert.NotEmpty(t, privPEM)
	assert.NotEmpty(t, pubStr)

	// Private key should be PEM encoded
	assert.Contains(t, string(privPEM), "-----BEGIN OPENSSH PRIVATE KEY-----")
	assert.Contains(t, string(privPEM), "-----END OPENSSH PRIVATE KEY-----")

	// Public key should be in authorized_keys format (ssh-ed25519 AAAA...)
	assert.True(t, strings.HasPrefix(pubStr, "ssh-ed25519 "))
}

func TestGenerateEd25519KeyPair_Unique(t *testing.T) {
	_, pub1, err := GenerateEd25519KeyPair()
	assert.NoError(t, err)

	_, pub2, err := GenerateEd25519KeyPair()
	assert.NoError(t, err)

	assert.NotEqual(t, pub1, pub2, "each call should generate a unique key pair")
}

func TestLoadOrGenerateKey_Generate(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")

	signer, pubStr, err := LoadOrGenerateKey(keyPath)
	assert.NoError(t, err)
	assert.NotNil(t, signer)
	assert.NotEmpty(t, pubStr)
	assert.True(t, strings.HasPrefix(pubStr, "ssh-ed25519 "))

	// Verify the key file was created
	_, err = os.Stat(keyPath)
	assert.NoError(t, err)

	// Verify file permissions (owner read/write only)
	info, err := os.Stat(keyPath)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadOrGenerateKey_Load(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")

	// Generate first
	signer1, pub1, err := LoadOrGenerateKey(keyPath)
	assert.NoError(t, err)

	// Load the same key
	signer2, pub2, err := LoadOrGenerateKey(keyPath)
	assert.NoError(t, err)

	// Should be the same key
	assert.Equal(t, pub1, pub2)
	assert.Equal(t,
		signer1.PublicKey().Marshal(),
		signer2.PublicKey().Marshal(),
	)
}

func TestLoadOrGenerateKey_NestedDir(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "deep", "nested", "dir", "test_key")

	signer, pubStr, err := LoadOrGenerateKey(keyPath)
	assert.NoError(t, err)
	assert.NotNil(t, signer)
	assert.NotEmpty(t, pubStr)

	// Verify the file was created in the nested directory
	_, err = os.Stat(keyPath)
	assert.NoError(t, err)
}

func TestLoadOrGenerateKey_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")

	// Write garbage to the key file
	err := os.WriteFile(keyPath, []byte("not a valid key"), 0600)
	assert.NoError(t, err)

	// Should fail to parse
	_, _, err = LoadOrGenerateKey(keyPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse existing private key")
}
