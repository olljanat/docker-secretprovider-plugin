package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type passwordResponse struct {
	PasswordID int    `json:"PasswordID"`
	Title      string `json:"Title"`
	Password   string `json:"Password"`
	ExpiryDate string `json:"ExpiryDate"`
}

type PasswordstateBackend struct {
	baseURL string
	apiKey  string
	listID  string
}

func NewPasswordstateBackend(baseURL, apiKey, listID string) *PasswordstateBackend {
	return &PasswordstateBackend{
		baseURL: baseURL,
		apiKey:  apiKey,
		listID:  listID,
	}
}

func (b *PasswordstateBackend) FetchSecret(secretName string) (*FetchSecretResponse, error) {
	url := fmt.Sprintf("%s/searchpasswords/%s?title=%s&PreventAuditing=true", b.baseURL, b.listID, url.QueryEscape(secretName))
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP request: %v", err)
	}
	req.Header.Set("APIKey", b.apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error searching for password: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response from Passwordstate: status code %d", resp.StatusCode)
	}

	var passwords []passwordResponse
	if err := json.NewDecoder(resp.Body).Decode(&passwords); err != nil {
		return nil, fmt.Errorf("error decoding password response: %v", err)
	}

	if len(passwords) == 0 {
		return nil, fmt.Errorf("no password found with title %q in list %q", secretName, b.listID)
	}
	password := passwords[0]

	expiry, err := time.Parse("2.1.2006", password.ExpiryDate)
	if err != nil {
		fmt.Printf("error parsing expiry date %v for secret %v : %v", password.ExpiryDate, secretName, err)
	}
	return &FetchSecretResponse{
		Value:     password.Password,
		UpdatedAt: time.Now(),
		ExpiresAt: expiry,
	}, nil
}

func (b *PasswordstateBackend) ListSecrets() ([]string, error) {
	url := fmt.Sprintf("%s/searchpasswords/%s?PreventAuditing=true", b.baseURL, b.listID)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP request for listing: %v", err)
	}
	req.Header.Set("APIKey", b.apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error listing passwords: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response from Passwordstate when listing: status code %d", resp.StatusCode)
	}

	var passwords []passwordResponse
	if err := json.NewDecoder(resp.Body).Decode(&passwords); err != nil {
		return nil, fmt.Errorf("error decoding password list response: %v", err)
	}

	var titles []string
	for _, p := range passwords {
		titles = append(titles, p.Title)
	}
	return titles, nil
}
