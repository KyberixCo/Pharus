package scraper

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type SecureFetcher struct {
	client   *http.Client
	dnsCache sync.Map // Map host -> net.IP (DNS Pinning)
}

func NewSecureFetcher() *SecureFetcher {
	sf := &SecureFetcher{}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
				port = "80"
			}

			var validIP net.IP

			// DNS Pinning lookup
			if pinned, ok := sf.dnsCache.Load(host); ok {
				validIP = pinned.(net.IP)
			} else {
				ips, err := net.LookupIP(host)
				if err != nil {
					return nil, fmt.Errorf("DNS lookup failed for %s: %w", host, err)
				}

				for _, ip := range ips {
					if isPrivateOrLoopbackIP(ip) {
						return nil, fmt.Errorf("SSRF protection: access to private/loopback IP %s is blocked", ip.String())
					}
					if validIP == nil {
						validIP = ip
					}
				}

				if validIP == nil {
					return nil, fmt.Errorf("SSRF protection: no valid public IP resolved for %s", host)
				}

				// Pin valid IP to host
				sf.dnsCache.Store(host, validIP)
			}

			// Dial pinned IP directly (DNS Pinning enforcement against Rebinding)
			pinnedAddr := net.JoinHostPort(validIP.String(), port)
			return dialer.DialContext(ctx, network, pinnedAddr)
		},
	}

	sf.client = &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			host := req.URL.Hostname()
			ips, err := net.LookupIP(host)
			if err != nil {
				return fmt.Errorf("redirect DNS lookup failed for %s: %w", host, err)
			}
			for _, ip := range ips {
				if isPrivateOrLoopbackIP(ip) {
					return fmt.Errorf("SSRF protection: redirect to private/loopback IP %s is blocked", ip.String())
				}
			}
			return nil
		},
	}

	return sf
}

func (f *SecureFetcher) Fetch(ctx context.Context, pageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Pharus-DeepResearch-Engine/1.0 (+https://github.com/KyberixCo/Pharus)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d fetching %s", resp.StatusCode, pageURL)
	}

	// Limit reading max 5 MB per page to avoid OOM
	lr := io.LimitReader(resp.Body, 5*1024*1024)
	return io.ReadAll(lr)
}

func isPrivateOrLoopbackIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Handle IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1)
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"100.64.0.0/10",
		"::1/128",
		"fe80::/10",
		"fc00::/7",
	}

	for _, cidr := range privateRanges {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}
