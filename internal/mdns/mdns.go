package mdns

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/brutella/dnssd"
	"github.com/mallardduck/dirio/internal/hostname"
	"github.com/mallardduck/dirio/internal/logging"
)

// Service represents an mDNS service registration for DirIO.
//
// The service uses a unique hostname format: <service-name>-<unique-id>.local
// This prevents conflicts with system mDNS responders (Bonjour/Avahi) and allows
// us to safely run our own mDNS responder on all platforms.
type Service struct {
	responder dnssd.Responder
	handles   []dnssd.ServiceHandle
	config    *Config
	log       *slog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
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
	if s.responder != nil {
		return fmt.Errorf("mDNS service already started")
	}

	// Get unique hostname from hostname package
	uniqueID := hostname.Identity()
	hostnameStr := fmt.Sprintf("%s-%s", s.config.ServiceName, uniqueID)

	s.log.Debug("creating dnssd service",
		"name", s.config.ServiceName,
		"hostname", hostnameStr,
		"port", s.config.Port)

	// Create dnssd configs for each service type
	configs := s.createServiceConfigs(hostnameStr)

	// Create responder
	responder, err := dnssd.NewResponder()
	if err != nil {
		return fmt.Errorf("failed to create dnssd responder: %w", err)
	}

	// Add all services and store handles
	handles := make([]dnssd.ServiceHandle, 0, len(configs))
	for _, config := range configs {
		service, err := dnssd.NewService(config)
		if err != nil {
			s.log.Error("failed to create service", "error", err, "config", config)
			continue
		}

		handle, err := responder.Add(service)
		if err != nil {
			return fmt.Errorf("failed to add service %s: %w", config.Name, err)
		}

		handles = append(handles, handle)

		s.log.Info("registered dnssd service",
			"name", service.Name,
			"type", service.Type,
			"host", service.Host,
			"port", service.Port)
	}

	// Store responder and handles
	s.responder = responder
	s.handles = handles

	// Create context for service lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	// Start responder in background
	go s.run()

	s.log.Info("mdns service started",
		"service", s.config.ServiceName,
		"hostname", hostnameStr+".local",
		"port", s.config.Port)

	return nil
}

// run starts the dnssd responder loop.
func (s *Service) run() {
	s.log.Debug("starting dnssd responder")

	err := s.responder.Respond(s.ctx)
	if err != nil && s.ctx.Err() == nil {
		s.log.Error("dnssd responder stopped with error", "error", err)
	} else {
		s.log.Debug("dnssd responder stopped")
	}
}

// Stop gracefully shuts down the mDNS service.
// This removes all service records (sending goodbye packets) before stopping the responder.
func (s *Service) Stop() error {
	if s.responder == nil {
		return nil
	}

	s.log.Debug("stopping mdns service, removing all service records")

	// Remove all service handles to send goodbye packets
	for _, handle := range s.handles {
		s.responder.Remove(handle)
	}

	s.log.Debug("service records removed, waiting for goodbye packets to be sent")

	// Give time for goodbye packets to be sent
	// This is important for clean mDNS behavior
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop the responder
	if s.cancel != nil {
		s.cancel()
	}

	// Clear references
	s.responder = nil
	s.handles = nil
	s.ctx = nil
	s.cancel = nil

	s.log.Info("mdns service stopped")
	return nil
}

// IsRunning returns true if the mDNS service is currently running.
func (s *Service) IsRunning() bool {
	return s.responder != nil
}

// GetAdvertisedHost returns the hostname being advertised via mDNS.
// Format: <service-name>-<unique-id>.local (e.g., "dirio-s3-abc123.local")
func (s *Service) GetAdvertisedHost() string {
	uniqueID := hostname.Identity()
	return fmt.Sprintf("%s-%s.local", s.config.ServiceName, uniqueID)
}

// createServiceConfigs creates dnssd.Config for each service type.
func (s *Service) createServiceConfigs(hostname string) []dnssd.Config {
	baseConfig := dnssd.Config{
		Host: hostname,
		Port: s.config.Port,
	}

	// Build TXT records
	if len(s.config.TXTRecords) > 0 {
		baseConfig.Text = s.config.TXTRecords
	}

	configs := []dnssd.Config{
		// S3 service
		{
			Name:   s.config.ServiceName,
			Type:   "_s3._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   baseConfig.Port,
			Text:   baseConfig.Text,
		},
		// HTTP service
		{
			Name:   s.config.ServiceName + " HTTP",
			Type:   "_http._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   baseConfig.Port,
			Text:   baseConfig.Text,
		},
	}

	// Add HTTPS if configured
	if s.config.HTTPSPort > 0 {
		configs = append(configs, dnssd.Config{
			Name:   s.config.ServiceName + " HTTPS",
			Type:   "_https._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   s.config.HTTPSPort,
			Text:   baseConfig.Text,
		})
	}

	return configs
}
