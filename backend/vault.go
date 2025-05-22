package backend

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

type secretDataResponse struct {
	Data struct {
		Data     map[string]string `json:"data"`
		Metadata struct {
			CreatedTime    string            `json:"created_time"`
			CustomMetadata map[string]string `json:"custom_metadata"`
		} `json:"metadata"`
	} `json:"data"`
}

type listKeysResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}

func NewVaultBackend(vaultAddr, path, token string) (*VaultBackend, error) {
	return &VaultBackend{
		client:    &http.Client{Timeout: 5 * time.Second},
		vaultAddr: vaultAddr,
		path:      path,
		token:     token,
	}, nil
}

// https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2#read-secret-version
func (b *VaultBackend) FetchSecret(secretName string) (*FetchSecretResponse, error) {
	url := fmt.Sprintf("%s/v1/%s/data/%s", b.vaultAddr, b.path, secretName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("X-Vault-Token", b.token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error reading secret %s: %v", secretName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("secret %s not found: status %d", secretName, resp.StatusCode)
	}

	var sdr secretDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&sdr); err != nil {
		return nil, fmt.Errorf("error decoding secret response: %v", err)
	}

	// extract the 'Secret' field, fallback to first entry if missing
	value, ok := sdr.Data.Data["Secret"]
	if !ok {
		for _, v := range sdr.Data.Data {
			value = v
			break
		}
	}

	// parse creation timestamp
	createdAt, err := time.Parse(time.RFC3339, sdr.Data.Metadata.CreatedTime)
	if err != nil {
		return nil, fmt.Errorf("error parsing created_time: %v", err)
	}

	// parse expiry from custom metadata
	var expiresAt time.Time
	if expiryStr, exists := sdr.Data.Metadata.CustomMetadata["ExpiryDate"]; exists && expiryStr != "" {
		expiresAt, err = time.Parse("2006-01-02", expiryStr)
		if err != nil {
			return nil, fmt.Errorf("error parsing ExpiryDate: %v", err)
		}
	}

	return &FetchSecretResponse{
		Value:     value,
		UpdatedAt: createdAt,
		ExpiresAt: expiresAt,
	}, nil
}

// https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2#list-secrets
func (b *VaultBackend) ListSecrets() ([]string, error) {
	url := fmt.Sprintf("%s/v1/%s/metadata?list=true", b.vaultAddr, b.path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %v", err)
	}
	req.Header.Set("X-Vault-Token", b.token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error listing secrets: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing secrets failed: status %d", resp.StatusCode)
	}
	var lkr listKeysResponse
	if err := json.NewDecoder(resp.Body).Decode(&lkr); err != nil {
		return nil, fmt.Errorf("error decoding list keys response: %v", err)
	}
	return lkr.Data.Keys, nil
}
