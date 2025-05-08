package backends

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

type AzureKeyVaultBackend struct {
	client *azsecrets.Client
}

func NewAzureKeyVaultBackend(keyVaultURL string) (*AzureKeyVaultBackend, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %v", err)
	}

	client, err := azsecrets.NewClient(keyVaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure Key Vault client: %v", err)
	}

	return &AzureKeyVaultBackend{client: client}, nil
}

func (b *AzureKeyVaultBackend) FetchSecret(secretName string, options map[string]string) (string, error) {
	resp, err := b.client.GetSecret(context.Background(), secretName, "", nil)
	if err != nil {
		return "", fmt.Errorf("error fetching secret %q from Azure Key Vault: %v", secretName, err)
	}
	return *resp.Value, nil
}
