package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/docker/go-plugins-helpers/volume"
	"github.com/olljanat/docker-secretprovider-plugin/backend"
	"github.com/sirupsen/logrus"
)

const (
	refreshInterval = 1 * time.Hour
	dbFile          = "secrets.json"
	npipeMaxBuf     = 4096
)

var (
	log       = logger()
	validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)
)

type SecretBackend interface {
	FetchSecret(secretName string) (*backend.FetchSecretResponse, error)
	ListSecrets() ([]string, error)
}

type volumeInfo struct {
	SecretName string
	UpdatedAt  time.Time
	ExpiresAt  time.Time
	Valid      bool
}

type VolumeDriver struct {
	volumes map[string]*volumeInfo
	backend SecretBackend
	mu      sync.RWMutex
}

type simpleFormatter struct{}

func NewVolumeDriver(backend SecretBackend) *VolumeDriver {
	d := &VolumeDriver{
		volumes: make(map[string]*volumeInfo),
		backend: backend,
	}

	if err := d.loadDB(); err != nil {
		log.Errorf("Failed to read database from disk: %v", err)
	}
	go d.startSecretRefresh()
	return d
}

func (d *VolumeDriver) startSecretRefresh() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for range ticker.C {
		d.mu.Lock()
		for name, vol := range d.volumes {
			if err := d.updateSecretFile(name, vol, false); err != nil {
				log.Errorf("Failed to update secret for volume %s: %v", name, err)
			}

			// Avoid API rate limiter issues by waiting 10 seconds between calls
			time.Sleep(10 * time.Second)
		}
		d.mu.Unlock()
	}
}

func (d *VolumeDriver) updateSecretFile(volumeName string, vol *volumeInfo, add bool) error {
	secretFile := filepath.Join(baseDir, volumeName)
	if _, err := os.Stat(secretFile); os.IsNotExist(err) && !add {
		return nil
	}

	secret, err := d.backend.FetchSecret(vol.SecretName)
	if err != nil {
		return fmt.Errorf("error fetching secret: %v", err)
	}

	if old, err := os.ReadFile(secretFile); err == nil && string(old) == secret.Value {
		return nil
	}
	if err := os.WriteFile(secretFile, []byte(secret.Value), 0644); err != nil {
		return fmt.Errorf("error writing secret %s: %v", volumeName, err)
	}
	vol.UpdatedAt = secret.UpdatedAt
	vol.ExpiresAt = secret.ExpiresAt
	d.saveDB()
	log.Printf("Updated secret for volume %s", volumeName)
	return nil
}

func (d *VolumeDriver) Create(r *volume.CreateRequest) error {
	return nil
}

func (d *VolumeDriver) List() (*volume.ListResponse, error) {
	names, err := d.backend.ListSecrets()
	if err != nil {
		log.Errorf("Failed to list secrets: %v", err)
	}

	d.mu.RLock()
	volumes := d.volumes
	d.mu.RUnlock()

	for _, name := range names {
		valid := true
		if _, exists := volumes[name]; !exists {
			if !validName.MatchString(name) {
				log.Warnf("Skipping invalid secret with name %q. Must match [a-z0-9][a-z0-9_.-]*", name)
				valid = false
			}
			d.mu.Lock()
			volumes[name] = &volumeInfo{
				SecretName: name,
				UpdatedAt:  time.Time{},
				Valid:      valid,
			}
			d.mu.Unlock()
		}
	}
	var vols []*volume.Volume
	for name := range volumes {
		v := volumes[name]
		if v.Valid {
			vols = append(vols, &volume.Volume{Name: name})
		}
	}
	resp := &volume.ListResponse{Volumes: vols}

	if runtime.GOOS == "windows" {
		if data, err := json.Marshal(resp); err == nil {
			if len(data) > npipeMaxBuf {
				log.Errorf("named pipe buffer too small: response size=%d bytes, max=%d; increase InBufferSize/OutBufferSize", len(data), npipeMaxBuf)
			}
		}
	}

	return resp, nil
}

func (d *VolumeDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	d.mu.RLock()
	volumes := d.volumes
	d.mu.RUnlock()

	vol, exists := volumes[r.Name]
	if !exists {
		d.List()
		vol, exists = volumes[r.Name]
		if !exists {
			return nil, fmt.Errorf("volume %s not found", r.Name)
		}
	}
	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name:      r.Name,
			CreatedAt: vol.UpdatedAt.Format(time.RFC3339),
		},
	}, nil
}

func (d *VolumeDriver) Remove(r *volume.RemoveRequest) error {
	return nil
}

func (d *VolumeDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	d.mu.RLock()
	volumes := d.volumes
	d.mu.RUnlock()

	_, exists := volumes[r.Name]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	secretFile := filepath.Join(baseDir, r.Name)
	return &volume.PathResponse{Mountpoint: secretFile}, nil
}

func (d *VolumeDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	d.mu.RLock()
	volumes := d.volumes
	d.mu.RUnlock()

	vol, exists := volumes[r.Name]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", r.Name)
	}
	secretFile := filepath.Join(baseDir, r.Name)

	if _, err := os.Stat(secretFile); os.IsNotExist(err) {
		d.mu.Lock()
		if err := d.updateSecretFile(r.Name, vol, true); err != nil {
			log.Errorf("Failed to update secret for volume %s: %v", r.Name, err)
		}
		d.mu.Unlock()
	}

	return &volume.MountResponse{Mountpoint: secretFile}, nil
}

func (d *VolumeDriver) Unmount(r *volume.UnmountRequest) error {
	d.mu.RLock()
	volumes := d.volumes
	d.mu.RUnlock()

	if _, exists := volumes[r.Name]; !exists {
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
	log.SetFormatter(&simpleFormatter{})

	backendType := os.Getenv("SECRET_BACKEND")
	if backendType == "" {
		log.Fatal("SECRET_BACKEND environment variable is required (azure, passwordstate, vault)")
	}

	var b SecretBackend
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
		b, err = backend.NewAzureKeyVaultBackend(keyVaultURL)
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
		b, err = backend.NewVaultBackend(vaultAddr, vaultPath, vaultToken)
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
		b = backend.NewPasswordstateBackend(baseURL, apiKey, listID)
	default:
		log.Fatalf("Unsupported backend: %s", backendType)
	}

	d := NewVolumeDriver(b)
	h := volume.NewHandler(d)

	log.Infof("Starting secret plugin with %s backend", backendType)
	serve(h)
}

func (d *VolumeDriver) loadDB() error {
	dbPath := filepath.Join(baseDir, dbFile)
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return nil
	}
	var m map[string]volumeInfo
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for name, info := range m {
		d.volumes[name] = &volumeInfo{
			SecretName: info.SecretName,
			UpdatedAt:  info.UpdatedAt,
			ExpiresAt:  info.ExpiresAt,
		}
	}
	return nil
}

func (d *VolumeDriver) saveDB() error {
	dbPath := filepath.Join(baseDir, dbFile)
	tmp := dbPath + ".tmp"
	m := make(map[string]volumeInfo, len(d.volumes))
	for name, v := range d.volumes {
		m[name] = volumeInfo{
			SecretName: v.SecretName,
			UpdatedAt:  v.UpdatedAt,
			ExpiresAt:  v.ExpiresAt,
		}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, dbPath)
}

func (f *simpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(entry.Message + ""), nil
}
