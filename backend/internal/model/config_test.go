package model

import "testing"

func TestServerConfigRejectsUnsafeAuthDefaults(t *testing.T) {

	tests := []struct {
		name string
		cfg  ServerConfig
	}{
		{
			name: "missing users",
			cfg: ServerConfig{
				JWTSecret:   "test-secret",
				Port:        80,
				MaxUploadMB: 1,
				ChunkSizeMB: 1,
			},
		},
		{
			name: "placeholder jwt secret",
			cfg: ServerConfig{
				JWTSecret:   "change-me-in-production",
				Users:       map[string]string{"admin": "correct-password"},
				Port:        80,
				MaxUploadMB: 1,
				ChunkSizeMB: 1,
			},
		},
		{
			name: "placeholder password",
			cfg: ServerConfig{
				JWTSecret:   "test-secret",
				Users:       map[string]string{"admin": "change-me-in-production"},
				Port:        80,
				MaxUploadMB: 1,
				ChunkSizeMB: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
