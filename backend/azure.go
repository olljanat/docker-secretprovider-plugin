package backend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type AzureKeyVaultBackend struct {
	tenantID     string
	clientID     string
	clientSecret string
	vaultURL     string
	httpClient   *http.Client
	token        string
	tokenExpiry  time.Time
	mu           sync.Mutex
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type secretResponse struct {
	Value      string `json:"value"`
	Attributes struct {
		Exp     int64 `json:"exp"`
		Updated int64 `json:"updated"`
	} `json:"attributes"`
}

type listResponse struct {
	Value []struct {
		ID string `json:"id"`
	} `json:"value"`
	NextLink string `json:"nextLink"`
}

func (sr *secretResponse) ExpiresAt() time.Time {
	return time.Unix(sr.Attributes.Exp, 0)
}

func (sr *secretResponse) UpdatedAt() time.Time {
	return time.Unix(sr.Attributes.Updated, 0)
}

func NewAzureKeyVaultBackend(vaultURL string) (*AzureKeyVaultBackend, error) {
	tid := os.Getenv("AZURE_TENANT_ID")
	cid := os.Getenv("AZURE_CLIENT_ID")
	csecret := os.Getenv("AZURE_CLIENT_SECRET")
	if tid == "" || cid == "" || csecret == "" {
		return nil, fmt.Errorf("AZURE_TENANT_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET are required")
	}
	return &AzureKeyVaultBackend{
		tenantID:     tid,
		clientID:     cid,
		clientSecret: csecret,
		vaultURL:     strings.TrimRight(vaultURL, "/"),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (b *AzureKeyVaultBackend) acquireToken() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Until(b.tokenExpiry) > time.Minute {
		return nil
	}
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", b.tenantID)
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", b.clientID)
	data.Set("client_secret", b.clientSecret)
	data.Set("scope", b.vaultURL+"/.default")
	resp, err := b.httpClient.PostForm(endpoint, data)
	if err != nil {
		return fmt.Errorf("failed to request token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("error decoding token response: %v", err)
	}
	b.token = tr.AccessToken
	b.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return nil
}

// https://learn.microsoft.com/en-us/rest/api/keyvault/secrets/get-secret/get-secret
func (b *AzureKeyVaultBackend) FetchSecret(secretName string) (*FetchSecretResponse, error) {
	if err := b.acquireToken(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/secrets/%s?api-version=7.4", b.vaultURL, url.PathEscape(secretName))
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+b.token)
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching secret %s: %v", secretName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch secret %s: status %d", secretName, resp.StatusCode)
	}
	var sr secretResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("error decoding secret %s: %v", secretName, err)
	}
	return &FetchSecretResponse{
		Value:     sr.Value,
		UpdatedAt: sr.UpdatedAt(),
		ExpiresAt: sr.ExpiresAt(),
	}, nil
}

// https://learn.microsoft.com/en-us/rest/api/keyvault/secrets/get-secrets/get-secrets
func (b *AzureKeyVaultBackend) ListSecrets() ([]string, error) {
	if err := b.acquireToken(); err != nil {
		return nil, err
	}
	var names []string
	next := fmt.Sprintf("%s/secrets?api-version=7.4", b.vaultURL)
	for next != "" {
		req, _ := http.NewRequest("GET", next, nil)
		req.Header.Set("Authorization", "Bearer "+b.token)
		resp, err := b.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error listing secrets: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var lr listResponse
		if err := json.Unmarshal(body, &lr); err != nil {
			return nil, fmt.Errorf("error unmarshalling list response: %v", err)
		}
		for _, item := range lr.Value {
			parts := strings.Split(item.ID, "/")
			slices.Reverse(parts)
			names = append(names, parts[0])
		}
		next = lr.NextLink
	}
	return names, nil
}
