package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// Guard enforces SSRF-safe outbound MCP connections. Platform policy rejects
// loopback/private/link-local/metadata targets unless AllowPrivate is set for
// development. The dialer re-checks the actual connected IP so DNS rebinding
// cannot bypass the URL check.
type Guard struct {
	AllowPrivate  bool
	AllowInsecure bool
	AllowedHosts  []string
	DialTimeout   time.Duration
}

func NewGuardFromEnv() *Guard {
	allowPrivate := truthy(os.Getenv("AGENT_MCP_ALLOW_PRIVATE"))
	return &Guard{
		AllowPrivate:  allowPrivate,
		AllowInsecure: truthy(os.Getenv("AGENT_MCP_ALLOW_INSECURE_HTTP")),
		AllowedHosts:  splitHosts(os.Getenv("AGENT_MCP_ALLOWED_HOSTS")),
		DialTimeout:   10 * time.Second,
	}
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitHosts(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ValidateURL rejects unsafe MCP endpoints. It performs DNS resolution, but the
// connection-time check remains authoritative.
func (g *Guard) ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid MCP URL")
	}
	if u.User != nil {
		return fmt.Errorf("MCP URL must not contain credentials")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("MCP URL must use https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("MCP URL must include a host")
	}
	if len(g.AllowedHosts) > 0 && !hostAllowed(host, g.AllowedHosts) {
		return fmt.Errorf("MCP host %q is not allowlisted", host)
	}
	if u.Scheme == "http" && !g.AllowInsecure {
		return fmt.Errorf("MCP URL must use https")
	}
	if g.AllowPrivate {
		return nil
	}
	ips, err := resolveHost(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("MCP host %q could not be resolved", host)
	}
	for _, ip := range ips {
		if privateIP(ip) {
			return fmt.Errorf("MCP URL must not target a private, loopback or link-local address")
		}
	}
	return nil
}

func hostAllowed(host string, allowed []string) bool {
	for _, entry := range allowed {
		if host == entry || strings.HasSuffix(host, "."+entry) {
			return true
		}
	}
	return false
}

func resolveHost(host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func privateIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// HTTPClient returns a client whose transport validates the actual remote IP
// before connecting and re-validates every redirect hop.
func (g *Guard) HTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialTimeout := g.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			return g.validateRemoteAddr(address)
		},
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many MCP redirects")
			}
			return g.ValidateURL(req.URL.String())
		},
	}
}

func (g *Guard) validateRemoteAddr(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid MCP dial address")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("invalid MCP dial IP")
	}
	if !g.AllowPrivate && privateIP(ip) {
		return fmt.Errorf("MCP connection to private/loopback address is blocked")
	}
	return nil
}
