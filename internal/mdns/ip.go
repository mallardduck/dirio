package mdns

import (
	"fmt"
	"net"
)

// InterfaceProvider is an interface for obtaining network interface information.
// This abstraction allows for easier testing with mock interfaces.
type InterfaceProvider interface {
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(iface *net.Interface) ([]net.Addr, error)
}

// SystemInterfaceProvider implements InterfaceProvider using the real net package.
type SystemInterfaceProvider struct{}

func (s SystemInterfaceProvider) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

func (s SystemInterfaceProvider) InterfaceAddrs(iface *net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// defaultProvider is the default interface provider using the system's network stack.
var defaultProvider InterfaceProvider = SystemInterfaceProvider{}

// GetOutboundIP returns the preferred outbound IP address of this machine.
// This answers the question: "How do we know the IP to use for mDNS record
// when binding to :9000 (all interfaces)?"
//
// The approach: Create a UDP "connection" to an external address. This doesn't
// actually send any packets, but it causes the OS to determine which local
// interface would be used for that route. We then extract the local address.
func GetOutboundIP() (net.IP, error) {
	// Use Google's DNS as a target - we never actually connect
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, fmt.Errorf("failed to determine outbound IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}

// GetLocalIP returns a suitable local IP address for mDNS advertisement.
// It tries the outbound IP method first, then falls back to interface enumeration.
func GetLocalIP() (net.IP, error) {
	return getLocalIPWithProvider(defaultProvider)
}

// getLocalIPWithProvider is the internal implementation that accepts an InterfaceProvider
// for testing purposes.
func getLocalIPWithProvider(provider InterfaceProvider) (net.IP, error) {
	// Try outbound IP method first (most reliable)
	ip, err := GetOutboundIP()
	if err == nil && ip != nil && !ip.IsLoopback() {
		return ip, nil
	}

	// Fallback: enumerate interfaces
	return getIPFromInterfacesWithProvider(provider)
}

// getIPFromInterfacesWithProvider enumerates network interfaces and returns the first
// suitable IPv4 address (non-loopback, up, unicast). It accepts an InterfaceProvider
// for testing purposes.
func getIPFromInterfacesWithProvider(provider InterfaceProvider) (net.IP, error) {
	ifaces, err := provider.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if !isValidInterface(iface) {
			continue
		}

		addrs, err := provider.InterfaceAddrs(&iface)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ip := extractValidIPv4(addr); ip != nil {
				return ip, nil
			}
		}
	}

	return nil, fmt.Errorf("no suitable IP address found")
}

// GetAllLocalIPs returns all suitable local IP addresses for mDNS advertisement.
// This can be used when advertising on multiple interfaces.
func GetAllLocalIPs() ([]net.IP, error) {
	return getAllLocalIPsWithProvider(defaultProvider)
}

// getAllLocalIPsWithProvider is the internal implementation that accepts
// an InterfaceProvider for testing purposes.
func getAllLocalIPsWithProvider(provider InterfaceProvider) ([]net.IP, error) {
	var ips []net.IP

	ifaces, err := provider.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if !isValidInterface(iface) {
			continue
		}

		addrs, err := provider.InterfaceAddrs(&iface)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ip := extractValidIPv4(addr); ip != nil {
				ips = append(ips, ip)
			}
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no suitable IP addresses found")
	}

	return ips, nil
}

// isValidInterface checks if an interface is suitable for mDNS advertisement.
// Returns true if the interface is not loopback and is up.
func isValidInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	if iface.Flags&net.FlagUp == 0 {
		return false
	}
	return true
}

// extractValidIPv4 extracts an IPv4 address from a net.Addr if it's valid
// for mDNS advertisement (non-loopback, IPv4).
func extractValidIPv4(addr net.Addr) net.IP {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	}

	// Skip nil, loopback, or non-IPv4
	if ip == nil || ip.IsLoopback() {
		return nil
	}

	return ip.To4()
}