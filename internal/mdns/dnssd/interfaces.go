package dnssd

import (
	"net"
	"strings"
)

// GetPrimaryInterface returns the primary network interface (the one with default route).
// This is typically the interface used for internet connectivity.
func GetPrimaryInterface() (*net.Interface, error) {
	// Try to determine which interface would be used for external connectivity
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// Fallback to first valid physical interface
		ifaces := GetPhysicalInterfaces()
		if len(ifaces) > 0 {
			return &ifaces[0], nil
		}
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Find the interface with this IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip.Equal(localAddr.IP) {
				return &iface, nil
			}
		}
	}

	// Fallback to first physical interface
	physical := GetPhysicalInterfaces()
	if len(physical) > 0 {
		return &physical[0], nil
	}

	return nil, net.ErrClosed
}

// GetPhysicalInterfaces returns all physical network interfaces, filtering out virtual ones.
func GetPhysicalInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	return FilterPhysicalInterfaces(ifaces)
}

// FilterPhysicalInterfaces filters out virtual and loopback interfaces.
func FilterPhysicalInterfaces(ifaces []net.Interface) []net.Interface {
	var physical []net.Interface
	for _, iface := range ifaces {
		if isPhysicalInterface(iface) {
			physical = append(physical, iface)
		}
	}
	return physical
}

// isPhysicalInterface checks if an interface is physical (not virtual, loopback, or down).
func isPhysicalInterface(iface net.Interface) bool {
	// Skip loopback
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}

	// Skip down interfaces
	if iface.Flags&net.FlagUp == 0 {
		return false
	}

	// Skip virtual interfaces by name pattern
	if isVirtualInterfaceName(iface.Name) {
		return false
	}

	return true
}

// isVirtualInterfaceName checks if an interface name matches known virtual interface patterns.
func isVirtualInterfaceName(name string) bool {
	virtualPrefixes := []string{
		"docker",  // Docker bridge
		"veth",    // Virtual ethernet (containers)
		"br-",     // Bridge
		"vmnet",   // VMware
		"vboxnet", // VirtualBox
		"virbr",   // libvirt
		"lxc",     // LXC
		"tun",     // TUN/TAP
		"tap",     // TAP
		"utun",    // macOS VPN
		"awdl",    // Apple Wireless Direct Link
		"llw",     // Low Latency WLAN
		"p2p",     // Point-to-point
		"bridge",  // Generic bridge
	}

	nameLower := strings.ToLower(name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}

	return false
}

// ValidateInterfaces validates a list of interface names and returns only valid ones.
// Returns the valid interface names and any invalid names that were filtered out.
func ValidateInterfaces(requestedNames []string) (valid []string, invalid []string) {
	if len(requestedNames) == 0 {
		return nil, nil
	}

	// Get all available interfaces
	allIfaces, err := net.Interfaces()
	if err != nil {
		return nil, requestedNames
	}

	// Create a map of available interface names
	available := make(map[string]bool)
	for _, iface := range allIfaces {
		available[iface.Name] = true
	}

	// Check each requested interface
	for _, name := range requestedNames {
		if available[name] {
			valid = append(valid, name)
		} else {
			invalid = append(invalid, name)
		}
	}

	return valid, invalid
}
