package dnssd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/brutella/dnssd"
)

// ServiceConfig holds configuration for creating a dnssd service.
type ServiceConfig struct {
	// Name is the service instance name (e.g., "dirio-s3")
	Name string

	// Hostname is the fully qualified hostname (e.g., "dirio-abc123" - no .local suffix)
	Hostname string

	// Port is the service port
	Port int

	// IPs are the IP addresses to advertise (for A/AAAA records)
	IPs []net.IP

	// Interfaces are the network interface names to bind to (e.g., ["en0"])
	// If empty, uses primary interface
	Interfaces []string

	// TXTRecords are additional TXT records for the service
	TXTRecords map[string]string

	// HTTPSPort is the HTTPS port (optional, if different from Port)
	HTTPSPort int
}

// Service wraps a brutella/dnssd responder for easier management.
type Service struct {
	responder dnssd.Responder
	handles   []dnssd.ServiceHandle
	ctx       context.Context
	cancel    context.CancelFunc
	log       *slog.Logger
}

// NewService creates and starts a new dnssd service.
func NewService(ctx context.Context, cfg *ServiceConfig, log *slog.Logger) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("service config is nil")
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("service name is required")
	}

	if cfg.Hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}

	if cfg.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	log.Debug("creating dnssd service",
		"name", cfg.Name,
		"hostname", cfg.Hostname,
		"port", cfg.Port,
		"ips", cfg.IPs,
		"interfaces", cfg.Interfaces)

	// Resolve interfaces
	interfaces := cfg.Interfaces
	if len(interfaces) == 0 {
		// Use primary interface by default
		primary, err := GetPrimaryInterface()
		if err != nil {
			log.Warn("failed to detect primary interface, using all interfaces", "error", err)
		} else {
			interfaces = []string{primary.Name}
			log.Debug("using primary interface", "interface", primary.Name)
		}
	} else {
		// Validate requested interfaces
		valid, invalid := ValidateInterfaces(interfaces)
		if len(invalid) > 0 {
			log.Warn("some requested interfaces are invalid and will be ignored",
				"invalid", invalid,
				"valid", valid)
		}
		interfaces = valid
	}

	// Create dnssd configs for each service type
	configs := createServiceConfigs(cfg, interfaces)

	// Create responder
	responder, err := dnssd.NewResponder()
	if err != nil {
		return nil, fmt.Errorf("failed to create dnssd responder: %w", err)
	}

	// Add all services and store handles
	var handles []dnssd.ServiceHandle
	for _, config := range configs {
		service, err := dnssd.NewService(config)
		if err != nil {
			log.Error("failed to create service", "error", err, "config", config)
			continue
		}

		handle, err := responder.Add(service)
		if err != nil {
			return nil, fmt.Errorf("failed to add service %s: %w", config.Name, err)
		}

		handles = append(handles, handle)

		log.Info("registered dnssd service",
			"name", service.Name,
			"type", service.Type,
			"host", service.Host,
			"port", service.Port,
			"interfaces", interfaces)
	}

	// Create service context
	svcCtx, cancel := context.WithCancel(ctx)

	svc := &Service{
		responder: responder,
		handles:   handles,
		ctx:       svcCtx,
		cancel:    cancel,
		log:       log,
	}

	// Start responder in background
	go svc.run()

	return svc, nil
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

// Stop gracefully stops the dnssd service.
// This removes all service records (sending goodbye packets) before stopping the responder.
func (s *Service) Stop() {
	s.log.Debug("stopping dnssd service, removing all service records")

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
}

// createServiceConfigs creates dnssd.Config for each service type.
func createServiceConfigs(cfg *ServiceConfig, interfaces []string) []dnssd.Config {
	baseConfig := dnssd.Config{
		Host:   cfg.Hostname,
		Port:   cfg.Port,
		IPs:    cfg.IPs,
		Ifaces: interfaces,
	}

	// Build TXT records
	if len(cfg.TXTRecords) > 0 {
		baseConfig.Text = cfg.TXTRecords
	}

	configs := []dnssd.Config{
		// S3 service
		{
			Name:   cfg.Name,
			Type:   "_s3._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   baseConfig.Port,
			IPs:    baseConfig.IPs,
			Ifaces: baseConfig.Ifaces,
			Text:   baseConfig.Text,
		},
		// HTTP service
		{
			Name:   cfg.Name + " HTTP",
			Type:   "_http._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   baseConfig.Port,
			IPs:    baseConfig.IPs,
			Ifaces: baseConfig.Ifaces,
			Text:   baseConfig.Text,
		},
	}

	// Add HTTPS if configured
	if cfg.HTTPSPort > 0 {
		configs = append(configs, dnssd.Config{
			Name:   cfg.Name + " HTTPS",
			Type:   "_https._tcp",
			Domain: "local",
			Host:   baseConfig.Host,
			Port:   cfg.HTTPSPort,
			IPs:    baseConfig.IPs,
			Ifaces: baseConfig.Ifaces,
			Text:   baseConfig.Text,
		})
	}

	return configs
}
