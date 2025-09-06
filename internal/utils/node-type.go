package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// NodeType represents whether the node should run as public or private
type NodeType string

// IPResponse represents the response from external IP services
type IPResponse struct {
	IP string `json:"ip"`
}

const (
	Public  NodeType = "public"
	Private NodeType = "private"
)

type NodeTypeManager struct {
	Type         NodeType        `json:"type"`
	LocalIP      string          `json:"local_ip"`
	ExternalIP   string          `json:"external_ip,omitempty"`
	Connectivity map[uint16]bool `json:"connectivity"`
	Timestamp    time.Time       `json:"timestamp"`
}

func NewNodeTypeManager() *NodeTypeManager {
	return &NodeTypeManager{}
}

// getLocalIP returns the local IP address of the machine
func (nt *NodeTypeManager) getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

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

			// Only consider IPv4 addresses
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable local IP found")
}

// isPrivateIP checks if the given IP is in private address ranges
func (nt *NodeTypeManager) isPrivateIP(ip string) bool {
	privateRanges := []string{
		`^10\.`,                      // 10.0.0.0/8
		`^172\.(1[6-9]|2\d|3[01])\.`, // 172.16.0.0/12
		`^192\.168\.`,                // 192.168.0.0/16
		`^127\.`,                     // 127.0.0.0/8 (loopback)
		`^169\.254\.`,                // 169.254.0.0/16 (link-local)
	}

	for _, pattern := range privateRanges {
		matched, err := regexp.MatchString(pattern, ip)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// getExternalIP fetches the external IP using a public service
func (nt *NodeTypeManager) getExternalIP() (string, error) {
	// Try multiple services for reliability
	services := []string{
		"https://api.ipify.org?format=json",
		"https://ipinfo.io/json",
		"https://httpbin.org/ip",
	}

	// Create HTTP client with disabled connection pooling to prevent UPnP issues
	transport := &http.Transport{
		DisableKeepAlives:     true,
		DisableCompression:    true,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   0,
		IdleConnTimeout:       1 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	// Ensure transport connections are closed after use
	defer func() {
		if transport != nil {
			transport.CloseIdleConnections()
		}
	}()

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		var ipResp IPResponse
		if err := json.NewDecoder(resp.Body).Decode(&ipResp); err != nil {
			continue
		}

		if ipResp.IP != "" {
			// Force close any remaining connections before returning
			transport.CloseIdleConnections()
			return ipResp.IP, nil
		}
	}

	return "", fmt.Errorf("could not determine external IP")
}

// detectNodeType automatically detects if the node should run as public or private
func (nt *NodeTypeManager) detectNodeType() (NodeType, error) {
	// Method 1: Get local IP
	localIP, err := nt.getLocalIP()
	if err != nil {
		// Could not determine local IP
		return Private, nil
	}

	// Method 2: Check if local IP is private
	if nt.isPrivateIP(localIP) {
		// Local IP is private, node type: private
		return Private, nil
	}

	// Method 3: Compare with external IP
	externalIP, err := nt.getExternalIP()
	if err != nil {
		// Could not fetch external IP
		// Defaulting to private due to error
		return Private, nil
	}

	if localIP == externalIP {
		// Local IP matches external IP, node type: public
		return Public, nil
	} else {
		// Local IP differs from external IP, node type: private
		return Private, nil
	}
}

// determineNodeType checks for manual override first, then auto-detects
func (nt *NodeTypeManager) determineNodeType() (NodeType, error) {
	// Check for manual override via environment variable
	if manualType := os.Getenv("NODE_TYPE"); manualType != "" {
		manualType = strings.ToLower(manualType)
		if manualType == "public" || manualType == "private" {
			// Using manual override
			return NodeType(manualType), nil
		}
	}

	// Otherwise auto-detect
	return nt.detectNodeType()
}

// isPortOpen checks if a port is open and accessible from outside
func (nt *NodeTypeManager) isPortOpen(port uint16) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	defer listener.Close()
	return true
}

// checkConnectivity performs additional connectivity checks
func (nt *NodeTypeManager) checkConnectivity(ports []uint16) map[uint16]bool {
	checks := make(map[uint16]bool)
	for _, port := range ports {
		checks[port] = nt.isPortOpen(port)
	}

	return checks
}

// NodeTypeConfig represents the configuration for the node
type NodeTypeConfig struct {
	Type         NodeType        `json:"type"`
	LocalIP      string          `json:"local_ip"`
	ExternalIP   string          `json:"external_ip,omitempty"`
	Connectivity map[uint16]bool `json:"connectivity"`
	Timestamp    time.Time       `json:"timestamp"`
}

// GetNodeTypeConfig returns complete node type configuration
func (nt *NodeTypeManager) GetNodeTypeConfig(ports []uint16) (*NodeTypeConfig, error) {
	nodeType, err := nt.determineNodeType()
	if err != nil {
		return nil, err
	}

	localIP, _ := nt.getLocalIP()
	var externalIP string
	if nodeType == Public {
		externalIP, _ = nt.getExternalIP()
	}

	config := &NodeTypeConfig{
		Type:         nodeType,
		LocalIP:      localIP,
		ExternalIP:   externalIP,
		Connectivity: nt.checkConnectivity(ports),
		Timestamp:    time.Now(),
	}

	return config, nil
}
