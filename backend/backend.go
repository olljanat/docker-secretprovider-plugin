package backend

import (
	"time"
)

type FetchSecretResponse struct {
	Value     string
	UpdatedAt time.Time
	ExpiresAt time.Time
}
