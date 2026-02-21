// aibox-credential-helper implements the git-credential protocol.
// It is configured via: git config --global credential.helper '/usr/local/bin/aibox-credential-helper'
//
// Git calls it with one argument: "get", "store", or "erase"
//
// For "get":
//
//	Reads protocol/host from stdin
//	Returns username + password (token) on stdout
//
// For "store" and "erase":
//
//	No-op (tokens managed by Vault/broker, not by git)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// credentialInput represents parsed git-credential protocol input.
type credentialInput struct {
	Protocol string
	Host     string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: aibox-credential-helper <get|store|erase>")
		os.Exit(1)
	}

	op := os.Args[1]
	switch op {
	case "get":
		if err := handleGet(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aibox-credential-helper: %v\n", err)
			os.Exit(1)
		}
	case "store", "erase":
		// No-op: tokens are managed by Vault/broker, not by git.
	default:
		fmt.Fprintf(os.Stderr, "aibox-credential-helper: unknown operation %q\n", op)
		os.Exit(1)
	}
}

// handleGet reads the git-credential protocol from r and writes a response to w.
func handleGet(r io.Reader, w io.Writer) error {
	input, err := parseInput(r)
	if err != nil {
		return fmt.Errorf("parsing input: %w", err)
	}

	token, err := resolveToken()
	if err != nil {
		return fmt.Errorf("resolving token: %w", err)
	}

	return writeOutput(w, input, token)
}

// parseInput reads key=value pairs from the git-credential protocol.
func parseInput(r io.Reader) (*credentialInput, error) {
	input := &credentialInput{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "protocol":
			input.Protocol = v
		case "host":
			input.Host = v
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return input, nil
}

// resolveToken obtains a git token from the best available source.
// Priority: AIBOX_GIT_TOKEN env var > Vault HTTP API.
func resolveToken() (string, error) {
	// Fallback mode: read from environment variable.
	if token := os.Getenv("AIBOX_GIT_TOKEN"); token != "" {
		return token, nil
	}

	// Full mode: fetch from Vault.
	vaultAddr := os.Getenv("AIBOX_VAULT_ADDR")
	if vaultAddr == "" {
		return "", fmt.Errorf("neither AIBOX_GIT_TOKEN nor AIBOX_VAULT_ADDR is set")
	}

	return fetchFromVault(vaultAddr)
}

// fetchFromVault calls the Vault HTTP API to get a short-lived git token.
func fetchFromVault(vaultAddr string) (string, error) {
	vaultToken := os.Getenv("AIBOX_VAULT_TOKEN")
	if vaultToken == "" {
		return "", fmt.Errorf("AIBOX_VAULT_TOKEN not set")
	}

	url := fmt.Sprintf("%s/v1/aibox/data/git-token", vaultAddr)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", vaultToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vault returned status %d: %s", resp.StatusCode, body)
	}

	var secret struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		return "", fmt.Errorf("decoding vault response: %w", err)
	}

	token, ok := secret.Data.Data["value"]
	if !ok || token == "" {
		return "", fmt.Errorf("vault secret has no 'value' key")
	}

	return token, nil
}

// writeOutput writes the git-credential protocol response.
func writeOutput(w io.Writer, input *credentialInput, token string) error {
	protocol := input.Protocol
	if protocol == "" {
		protocol = "https"
	}
	host := input.Host
	if host == "" {
		host = "git.internal"
	}

	fmt.Fprintf(w, "protocol=%s\n", protocol)
	fmt.Fprintf(w, "host=%s\n", host)
	fmt.Fprintf(w, "username=x-token\n")
	fmt.Fprintf(w, "password=%s\n", token)

	return nil
}
