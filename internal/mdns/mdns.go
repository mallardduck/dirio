package mdns

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mallardduck/dirio/internal/hostname"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/mdns/dnssd"
)

// Service represents an mDNS service registration for DirIO.
//
// The service uses a unique hostname format: <service-name>-<unique-id>.local
// This prevents conflicts with system mDNS responders (Bonjour/Avahi) and allows
// us to safely run our own mDNS responder on all platforms.
type Service struct {
	service *dnssd.Service
	config  *Config
	log     *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
}

// Config holds configuration for mDNS service registration.
type Config struct {
	// ServiceName is the mDNS service name (e.g., "dirio-s3")
	ServiceName string

	// Port is the HTTP port number
	Port int

	// HTTPSPort is the HTTPS port (optional)
	HTTPSPort int

	// TXTRecords are additional TXT records for service discovery
	TXTRecords map[string]string
}

// New creates a new mDNS service but does not start it.
func New(cfg *Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = "dirio-s3"
	}

	if cfg.Port == 0 {
		cfg.Port = 9000
	}

	return &Service{
		config: cfg,
		log:    logging.Component("mdns"),
	}, nil
}

// Start begins advertising the mDNS service on the local network.
//
// The service uses a unique hostname format: <service-name>-<unique-id>.local
// (e.g., "dirio-s3-abc123.local") which prevents conflicts with system
// mDNS responders.
//
// Service discovery records (_http._tcp, _s3._tcp) point to this unique hostname,
// allowing clients to discover and connect to the service.
func (s *Service) Start() error {
	if s.service != nil {
		return fmt.Errorf("mDNS service already started")
	}

	// Get unique hostname from hostname package
	uniqueID := hostname.Identity()
	hostnameStr := fmt.Sprintf("%s-%s", s.config.ServiceName, uniqueID)

	s.log.Debug("using unique hostname for mDNS",
		"hostname", hostnameStr,
		"unique_id", uniqueID)

	// Create context for service lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Configure dnssd service
	svcCfg := &dnssd.ServiceConfig{
		Name:       s.config.ServiceName,
		Hostname:   hostnameStr,
		Port:       s.config.Port,
		HTTPSPort:  s.config.HTTPSPort,
		TXTRecords: s.config.TXTRecords,
	}

	// Create and start dnssd service
	service, err := dnssd.NewService(ctx, svcCfg, s.log)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start mDNS service: %w", err)
	}

	s.service = service

	s.log.Info("mdns service registered",
		"service", s.config.ServiceName,
		"hostname", hostnameStr+".local",
		"port", s.config.Port)

	return nil
}

// Stop gracefully shuts down the mDNS service.
func (s *Service) Stop() error {
	if s.service == nil {
		return nil
	}

	s.log.Info("stopping mdns service", "service", s.config.ServiceName)

	// Stop the dnssd service
	s.service.Stop()

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Clear references
	s.service = nil
	s.ctx = nil
	s.cancel = nil

	s.log.Debug("mdns service stopped")
	return nil
}

// IsRunning returns true if the mDNS service is currently running.
func (s *Service) IsRunning() bool {
	return s.service != nil
}

// GetAdvertisedHost returns the hostname being advertised via mDNS.
// Format: <service-name>-<unique-id>.local (e.g., "dirio-s3-abc123.local")
func (s *Service) GetAdvertisedHost() string {
	uniqueID := hostname.Identity()
	return fmt.Sprintf("%s-%s.local", s.config.ServiceName, uniqueID)
}
