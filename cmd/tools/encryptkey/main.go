// encryptkey is a CLI helper that encrypts an LLM API key for safe storage in config.yaml.
//
// Usage:
//
//	export KUBEMINDS_MASTER_KEY=$(openssl rand -hex 32)  # generate once, store securely
//	make encrypt-key KEY=sk-xxxx                         # prints the enc:aes256:... string
//
// The printed string can be pasted directly as the apiKey value in config.yaml.
// The application will decrypt it automatically at startup using KUBEMINDS_MASTER_KEY.
package main

import (
	"fmt"
	"os"

	"kubeminds/internal/crypto"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "" {
		fmt.Fprintln(os.Stderr, "Usage: encryptkey <plaintext-api-key>")
		fmt.Fprintln(os.Stderr, "       KUBEMINDS_MASTER_KEY must be set (64 hex chars)")
		os.Exit(1)
	}

	plaintext := os.Args[1]

	// Read the master key from the environment variable.
	key, err := crypto.MasterKeyFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Generate a master key with: openssl rand -hex 32")
		os.Exit(1)
	}

	encrypted, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Encryption failed: %v\n", err)
		os.Exit(1)
	}

	// Print just the encrypted value — ready to paste into config.yaml.
	fmt.Println(encrypted)
}
