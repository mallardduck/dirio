package mdns

import (
	"testing"
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
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && svc == nil {
				t.Error("New() returned nil service without error")
			}
		})
	}
}

func TestServiceDefaults(t *testing.T) {
	cfg := &Config{}
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Check that defaults were applied
	if svc.config.ServiceName != "dirio-s3" {
		t.Errorf("default ServiceName = %q, want %q", svc.config.ServiceName, "dirio-s3")
	}
	if svc.config.Port != 9000 {
		t.Errorf("default Port = %d, want %d", svc.config.Port, 9000)
	}
}

func TestGetAdvertisedHost(t *testing.T) {
	svc, err := New(&Config{ServiceName: "my-service"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	host := svc.GetAdvertisedHost()
	if host != "my-service.local" {
		t.Errorf("GetAdvertisedHost() = %q, want %q", host, "my-service.local")
	}
}

func TestIsRunning(t *testing.T) {
	svc, err := New(&Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if svc.IsRunning() {
		t.Error("IsRunning() = true before Start()")
	}
}

func TestStartStop(t *testing.T) {
	// Skip if no network available
	_, err := GetLocalIP()
	if err != nil {
		t.Skip("No network available for mDNS test")
	}

	svc, err := New(&Config{
		ServiceName: "dirio-test",
		Port:        19000, // Use a non-standard port to avoid conflicts
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start the service
	if err := svc.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !svc.IsRunning() {
		t.Error("IsRunning() = false after Start()")
	}

	// Starting again should fail
	if err := svc.Start(); err == nil {
		t.Error("Start() should fail when already running")
	}

	// Stop the service
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if svc.IsRunning() {
		t.Error("IsRunning() = true after Stop()")
	}

	// Stop should be idempotent
	if err := svc.Stop(); err != nil {
		t.Errorf("Stop() should be idempotent, got error: %v", err)
	}
}