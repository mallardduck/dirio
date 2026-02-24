package mdns

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "empty config uses defaults",
			config:  &Config{},
			wantErr: false,
		},
		{
			name: "custom config",
			config: &Config{
				ServiceName: "test-service",
				Port:        8080,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, svc)
		})
	}
}

func TestServiceDefaults(t *testing.T) {
	cfg := &Config{}
	svc, err := New(cfg)
	require.NoError(t, err)

	// Check that defaults were applied
	assert.Equal(t, "dirio-s3", svc.config.ServiceName)
	assert.Equal(t, 9000, svc.config.Port)
}

func TestGetAdvertisedHost(t *testing.T) {
	svc, err := New(&Config{ServiceName: "my-service"})
	require.NoError(t, err)

	host := svc.GetAdvertisedHost()

	// Should be in format: my-service-{unique-id}.local
	assert.Contains(t, host, "my-service-")
	assert.Contains(t, host, ".local")
	assert.NotEqual(t, "my-service.local", host, "Should include unique ID component")
}

func TestIsRunning(t *testing.T) {
	svc, err := New(&Config{})
	require.NoError(t, err)

	assert.False(t, svc.IsRunning())
}

func TestStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	svc, err := New(&Config{
		ServiceName: "dirio-test",
		Port:        19000, // Use a non-standard port to avoid conflicts
	})
	require.NoError(t, err)

	// Start the service
	require.NoError(t, svc.Start(context.Background()))
	assert.True(t, svc.IsRunning())

	// Starting again should fail
	require.Error(t, svc.Start(context.Background()))

	// Stop the service
	require.NoError(t, svc.Stop())
	assert.False(t, svc.IsRunning())

	// Stop should be idempotent
	assert.NoError(t, svc.Stop())
}
