package backends

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type VaultBackend struct {
	client    *http.Client
	vaultAddr string
	path      string
	token     string
}

func NewVaultBackend(vaultAddr, path, token string) (*VaultBackend, error) {
	return &VaultBackend{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		vaultAddr: vaultAddr,
		path:      path,
		token:     token,
	}, nil
}

func (b *VaultBackend) FetchSecret(secretName string, options map[string]string) (string, error) {
	url := fmt.Sprintf("%s/v1/%s/data/%s", b.vaultAddr, b.path, secretName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request for Vault: %v", err)
	}
	req.Header.Set("X-Vault-Token", b.token)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error reading secret from Vault at %q: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("secret with name %s does not exist", secretName)
	}

	var result struct {
		Data struct {
			Data struct {
				Secret string `json:"secret"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("error decoding Vault response at %q: %v", url, err)
	}

	if result.Data.Data.Secret == "" {
		return "", fmt.Errorf("no secret found at %q", url)
	}

	return result.Data.Data.Secret, nil
}
