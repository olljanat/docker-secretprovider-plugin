package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/olljanat/docker-secretprovider-plugin/backends"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"
)

const (
	prefix          = "secret-"
	refreshInterval = 1 * time.Hour
)

var (
	log = logrus.New()
)

type SecretBackend interface {
	FetchSecret(secretName string, options map[string]string) (string, error)
}

type VolumeDriver struct {
	volumes map[string]*volumeInfo
	backend SecretBackend
	mu      sync.RWMutex
}

type volumeInfo struct {
	SecretName  string
	Options     map[string]string
	LastUpdated time.Time
}

func NewVolumeDriver(backend SecretBackend) *VolumeDriver {
	d := &VolumeDriver{
		volumes: make(map[string]*volumeInfo),
		backend: backend,
	}
	go d.startSecretRefresh()
	return d
}

func (d *VolumeDriver) startSecretRefresh() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			for name, vol := range d.volumes {
				if err := d.updateSecretFile(name, vol); err != nil {
					log.Errorf("Failed to update secret for volume %s: %v", name, err)
				}
			}
			d.mu.Unlock()
		}
	}
}

func (d *VolumeDriver) updateSecretFile(secretName string, vol *volumeInfo) error {
	secret, err := d.backend.FetchSecret(vol.SecretName, vol.Options)
	if err != nil {
		return fmt.Errorf("error fetching secret: %v", err)
	}

	secretFile := filepath.Join(baseDir, secretName)
	if err := os.WriteFile(secretFile, []byte(secret), 0644); err != nil {
		return fmt.Errorf("error writing secret to %s: %v", secretFile, err)
	}

	vol.LastUpdated = time.Now()
	log.Infof("Updated secret for volume %s", secretName)
	return nil
}

func (d *VolumeDriver) Create(r *volume.CreateRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	secretName := ""
	if strings.HasPrefix(r.Name, prefix) {
		secretName = strings.TrimPrefix(r.Name, prefix)
	} else {
		return fmt.Errorf("volume name '%s' does not have the required prefix '%s'", r.Name, prefix)
	}

	vol := &volumeInfo{
		SecretName: secretName,
		Options:    r.Options,
	}
	d.volumes[r.Name] = vol

	secretFile := filepath.Join(baseDir, r.Name)
	if err := d.updateSecretFile(r.Name, vol); err != nil {
		os.Remove(secretFile)
		delete(d.volumes, r.Name)
		return err
	}

	log.Infof("Created volume %s", r.Name)
	return nil
}

func (d *VolumeDriver) List() (*volume.ListResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var vols []*volume.Volume
	for name, _ := range d.volumes {
		vols = append(vols, &volume.Volume{
			Name: name,
		})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *VolumeDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.volumes[r.Name]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name: r.Name,
		},
	}, nil
}

func (d *VolumeDriver) Remove(r *volume.RemoveRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, exists := d.volumes[r.Name]
	if !exists {
		return fmt.Errorf("volume %s not found", r.Name)
	}

	secretFile := filepath.Join(baseDir, r.Name)
	if err := os.Remove(secretFile); err != nil {
		return fmt.Errorf("failed to remove secret %s: %v", secretFile, err)
	}

	delete(d.volumes, r.Name)
	log.Infof("Removed volume %s", r.Name)
	return nil
}

func (d *VolumeDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.volumes[r.Name]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	secretFile := filepath.Join(baseDir, r.Name)
	return &volume.PathResponse{Mountpoint: secretFile}, nil
}

func (d *VolumeDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.volumes[r.Name]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	secretFile := filepath.Join(baseDir, r.Name)
	return &volume.MountResponse{Mountpoint: secretFile}, nil
}

func (d *VolumeDriver) Unmount(r *volume.UnmountRequest) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, exists := d.volumes[r.Name]; !exists {
		return fmt.Errorf("volume %s not found", r.Name)
	}
	return nil
}

func (d *VolumeDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{Scope: "local"},
	}
}

func main() {
	backendType := os.Getenv("SECRET_BACKEND")
	if backendType == "" {
		log.Fatal("SECRET_BACKEND environment variable is required (azure, passwordstate, vault)")
	}

	var backend SecretBackend
	var err error

	switch backendType {
	case "azure":
		azureTenantID := os.Getenv("AZURE_TENANT_ID")
		if azureTenantID == "" {
			log.Fatal("AZURE_TENANT_ID environment variable is required")
		}
		azureClientID := os.Getenv("AZURE_CLIENT_ID")
		if azureClientID == "" {
			log.Fatal("AZURE_CLIENT_ID environment variable is required")
		}
		azureClientSecret := os.Getenv("AZURE_CLIENT_SECRET")
		if azureClientSecret == "" {
			log.Fatal("AZURE_CLIENT_SECRET environment variable is required")
		}
		keyVaultURL := os.Getenv("AZURE_KEYVAULT_URL")
		if keyVaultURL == "" {
			log.Fatal("AZURE_KEYVAULT_URL environment variable is required")
		}
		backend, err = backends.NewAzureKeyVaultBackend(keyVaultURL)
		if err != nil {
			log.Fatalf("Failed to initialize Azure Key Vault backend: %v", err)
		}

	case "vault":
		vaultAddr := os.Getenv("VAULT_ADDR")
		if vaultAddr == "" {
			log.Fatal("VAULT_ADDR environment variable is required")
		}
		vaultPath := os.Getenv("VAULT_PATH")
		if vaultPath == "" {
			log.Fatal("VAULT_PATH environment variable is required")
		}
		vaultToken := os.Getenv("VAULT_TOKEN")
		if vaultToken == "" {
			log.Fatal("VAULT_TOKEN environment variable is required")
		}
		backend, err = backends.NewVaultBackend(vaultAddr, vaultPath, vaultToken)
		if err != nil {
			log.Fatalf("Failed to initialize HashiCorp Vault backend: %v", err)
		}
	case "passwordstate":
		baseURL := os.Getenv("PASSWORDSTATE_BASE_URL")
		if baseURL == "" {
			log.Fatal("PASSWORDSTATE_BASE_URL environment variable is required")
		}
		apiKey := os.Getenv("PASSWORDSTATE_API_KEY")
		if apiKey == "" {
			log.Fatal("PASSWORDSTATE_API_KEY environment variable is required")
		}
		listID := os.Getenv("PASSWORDSTATE_LIST_ID")
		if listID == "" {
			log.Fatal("PASSWORDSTATE_LIST_ID environment variable is required")
		}
		backend = backends.NewPasswordstateBackend(baseURL, apiKey, listID)
	default:
		log.Fatalf("Unsupported backend: %s", backendType)
	}

	d := NewVolumeDriver(backend)
	h := volume.NewHandler(d)

	log.Infof("Starting secrets plugin with %s backend", backendType)
	serve(h)
}
