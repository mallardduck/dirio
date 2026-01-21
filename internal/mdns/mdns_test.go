package mdns

import (
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
			assert := assert.New(t)
			svc, err := New(tt.config)
			if tt.wantErr {
				assert.Error(err)
				return
			}
			assert.NoError(err)
			assert.NotNil(svc)
		})
	}
}

func TestServiceDefaults(t *testing.T) {
	assert := assert.New(t)
	cfg := &Config{}
	svc, err := New(cfg)
	require.NoError(t, err)

	// Check that defaults were applied
	assert.Equal("dirio-s3", svc.config.ServiceName)
	assert.Equal(9000, svc.config.Port)
}

func TestGetAdvertisedHost(t *testing.T) {
	svc, err := New(&Config{ServiceName: "my-service"})
	require.NoError(t, err)

	host := svc.GetAdvertisedHost()
	assert.Equal(t, "my-service.local", host)
}

func TestIsRunning(t *testing.T) {
	svc, err := New(&Config{})
	require.NoError(t, err)

	assert.False(t, svc.IsRunning())
}

func TestStartStop(t *testing.T) {
	// Skip if no network available
	_, err := GetLocalIP()
	if err != nil {
		t.Skip("No network available for mDNS test")
	}

	assert := assert.New(t)
	svc, err := New(&Config{
		ServiceName: "dirio-test",
		Port:        19000, // Use a non-standard port to avoid conflicts
	})
	require.NoError(t, err)

	// Start the service
	require.NoError(t, svc.Start())
	assert.True(svc.IsRunning())

	// Starting again should fail
	assert.Error(svc.Start())

	// Stop the service
	require.NoError(t, svc.Stop())
	assert.False(svc.IsRunning())

	// Stop should be idempotent
	assert.NoError(svc.Stop())
}