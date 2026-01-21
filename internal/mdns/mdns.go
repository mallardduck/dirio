package mdns

import (
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/hashicorp/mdns"
	"github.com/mallardduck/dirio/internal/logging"
)

// Service represents an mDNS service registration for DirIO.
type Service struct {
	server *mdns.Server
	config *Config
	log    *slog.Logger
}

// Config holds configuration for mDNS service registration.
type Config struct {
	// ServiceName is the mDNS service name (without .local suffix).
	// Default: "dirio-s3"
	ServiceName string

	// Port is the port number the S3 service is listening on.
	Port int

	// IPs are the IP addresses to advertise. If nil, auto-detected.
	IPs []net.IP
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
// The service will be advertised as:
//   - Name: <ServiceName>.local (e.g., dirio-s3.local)
//   - Type: _http._tcp (standard HTTP service type)
//
// Clients can discover the service using mDNS and connect to the
// advertised IP and port.
func (s *Service) Start() error {
	if s.server != nil {
		return fmt.Errorf("mDNS service already started")
	}

	// Get IPs to advertise
	ips := s.config.IPs
	if len(ips) == 0 {
		// Auto-detect local IP
		ip, err := GetLocalIP()
		if err != nil {
			return fmt.Errorf("failed to detect local IP for mDNS: %w", err)
		}
		ips = []net.IP{ip}
	}

	// Get hostname for the service info
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "dirio"
	}

	// Create service info
	// The service name becomes: <instance>._http._tcp.local
	// The host becomes: <ServiceName>.local
	info := []string{
		"DirIO S3-compatible storage",
		"version=1.0",
	}

	service, err := mdns.NewMDNSService(
		hostname,                       // Instance name
		"_http._tcp",                   // Service type
		"",                             // Domain (empty for .local)
		s.config.ServiceName+".local.", // Host name
		s.config.Port,                  // Port
		ips,                            // IPs
		info,                           // TXT records
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	// Create mDNS server config with verbosity-aware logging
	// In quiet mode, suppress noisy error messages from malformed packets
	serverConfig := &mdns.Config{
		Zone:   service,
		Logger: logging.StdLogger("mdns"),
	}

	// Create and start the server
	server, err := mdns.NewServer(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to start mDNS server: %w", err)
	}

	s.server = server

	s.log.Info("mdns service registered",
		"host", s.config.ServiceName+".local",
		"ips", ips,
		"port", s.config.Port,
	)

	return nil
}

// Stop gracefully shuts down the mDNS service.
func (s *Service) Stop() error {
	if s.server == nil {
		return nil
	}

	s.log.Info("stopping mdns service", "host", s.config.ServiceName+".local")

	err := s.server.Shutdown()
	s.server = nil

	if err != nil {
		return fmt.Errorf("failed to stop mDNS server: %w", err)
	}

	return nil
}

// IsRunning returns true if the mDNS service is currently running.
func (s *Service) IsRunning() bool {
	return s.server != nil
}

// GetAdvertisedHost returns the hostname being advertised via mDNS.
func (s *Service) GetAdvertisedHost() string {
	return s.config.ServiceName + ".local"
}
