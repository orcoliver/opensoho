package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// GenerateEd25519KeyPair generates a new Ed25519 SSH key pair.
// Returns the private key (PEM encoded) and public key (OpenSSH authorized_keys format).
func GenerateEd25519KeyPair() (privateKeyPEM []byte, publicKeyStr string, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate ed25519 key: %w", err)
	}

	// Marshal private key to PEM
	privKeyBytes, err := ssh.MarshalPrivateKey(privKey, "opensoho server key")
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal private key: %w", err)
	}
	privateKeyPEM = pem.EncodeToMemory(privKeyBytes)

	// Create SSH public key
	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create ssh public key: %w", err)
	}
	publicKeyStr = string(ssh.MarshalAuthorizedKey(sshPubKey))

	return privateKeyPEM, publicKeyStr, nil
}

// LoadOrGenerateKey loads an existing SSH key from path, or generates a new one
// if it doesn't exist. Returns the ssh.Signer for authentication and the public
// key string in authorized_keys format.
func LoadOrGenerateKey(path string) (ssh.Signer, string, error) {
	// Try to load existing key
	if data, err := os.ReadFile(path); err == nil {
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, "", fmt.Errorf("failed to parse existing private key at %s: %w", path, err)
		}
		pubKeyStr := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
		return signer, pubKeyStr, nil
	}

	// Generate new key pair
	privateKeyPEM, publicKeyStr, err := GenerateEd25519KeyPair()
	if err != nil {
		return nil, "", err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create directory for ssh key: %w", err)
	}

	// Write private key to disk
	if err := os.WriteFile(path, privateKeyPEM, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to write private key to %s: %w", path, err)
	}

	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse generated private key: %w", err)
	}

	return signer, publicKeyStr, nil
}
