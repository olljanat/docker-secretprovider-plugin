package backends

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type passwordResponse struct {
	PasswordID int    `json:"PasswordID"`
	Title      string `json:"Title"`
	Password   string `json:"Password"`
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

func (b *PasswordstateBackend) FetchSecret(secretName string, options map[string]string) (string, error) {
	url := fmt.Sprintf("%s/searchpasswords/%s?title=%s&PreventAuditing=true", b.baseURL, b.listID, url.QueryEscape(secretName))
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating HTTP request: %v", err)
	}
	req.Header.Set("APIKey", b.apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error searching for password: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected response from Passwordstate: status code %d", resp.StatusCode)
	}

	var passwords []passwordResponse
	if err := json.NewDecoder(resp.Body).Decode(&passwords); err != nil {
		return "", fmt.Errorf("error decoding password response: %v", err)
	}

	if len(passwords) == 0 {
		return "", fmt.Errorf("no password found with title %q in list %q", secretName, b.listID)
	}
	password := passwords[0]
	return password.Password, nil
}
