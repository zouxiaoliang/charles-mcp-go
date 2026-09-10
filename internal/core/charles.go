package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Charles struct {
	BaseURL, User, Password, ProxyURL string
	CLI                               string
	client                            *http.Client
}

func NewCharles(base, proxy, user, password string, timeout time.Duration) (*Charles, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid Charles URL")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	if proxy != "" {
		p, err := url.Parse(proxy)
		if err != nil || p.Host == "" || (p.Scheme != "http" && p.Scheme != "https") {
			return nil, fmt.Errorf("invalid Charles proxy URL")
		}
		tr.Proxy = http.ProxyURL(p)
	}
	return &Charles{BaseURL: strings.TrimRight(base, "/"), ProxyURL: proxy, User: user, Password: password, client: &http.Client{Transport: tr, Timeout: timeout}}, nil
}
func (c *Charles) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to Charles (enable Web Interface and check proxy/auth): %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Charles %s returned HTTP %d", path, res.StatusCode)
	}
	return readBounded(res.Body, MaxInputBytes)
}
func (c *Charles) Recording(ctx context.Context) (bool, error) {
	b, err := c.Get(ctx, "/recording/")
	if err != nil {
		return false, err
	}
	s := string(b)
	if strings.Contains(s, "Status: Recording Stopped") {
		return false, nil
	}
	if strings.Contains(s, "Status: Recording") {
		return true, nil
	}
	return false, fmt.Errorf("unrecognized Charles recording status page; check Web Interface authentication")
}
func (c *Charles) Record(ctx context.Context, on bool) error {
	p := "/recording/stop"
	if on {
		p = "/recording/start"
	}
	_, err := c.Get(ctx, p)
	return err
}
func (c *Charles) Throttle(ctx context.Context, preset string) error {
	p := strings.ToLower(preset)
	if p == "off" || p == "deactivate" {
		_, err := c.Get(ctx, "/throttling/deactivate")
		return err
	}
	presets := map[string]string{"3g": "3G", "4g": "4G", "5g": "5G", "fibre": "100 Mbps Fibre", "100mbps": "100 Mbps Fibre", "56k": "56 kbps Modem", "256k": "256 kbps ISDN/DSL"}
	if v, ok := presets[p]; ok {
		preset = v
	}
	if preset == "" {
		return fmt.Errorf("preset is required")
	}
	_, err := c.Get(ctx, "/throttling/activate?preset="+url.QueryEscape(preset))
	return err
}
func (c *Charles) Snapshot(ctx context.Context, format, capture string) ([]Entry, error) {
	p := "/session/export-xml"
	switch format {
	case "xml":
	case "native":
		p = "/session/download"
	case "json":
		p = "/session/export-json"
	default:
		return nil, fmt.Errorf("snapshot format must be xml, native or json")
	}
	b, err := c.Get(ctx, p)
	if err != nil {
		return nil, err
	}
	entries, parseErr := ParseSession(b, format, capture)
	var sizeErr *SizeLimitError
	if errors.As(parseErr, &sizeErr) {
		return nil, parseErr
	}
	if parseErr == nil || format != "native" {
		return entries, parseErr
	}
	f, err := os.CreateTemp("", "charles-live-*.chls")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	_, entries, err = ImportFile(ctx, path, "native", c.CLI)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].ID = ""
		entries[i].Normalize(capture, i+1)
	}
	return entries, nil
}
func (c *Charles) Close() { c.client.CloseIdleConnections() }

// Authentication and timeouts do not prove Charles is closed. Only an explicit
// connection refusal permits restoration of files that Charles may overwrite.
func (c *Charles) EnsureStopped(ctx context.Context) error {
	endpoint := c.ProxyURL
	if endpoint == "" {
		endpoint = c.BaseURL
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err == nil {
		conn.Close()
		return fmt.Errorf("Charles endpoint is still listening; quit Charles before restoring configuration")
	}
	if isConnectionRefused(err) {
		return nil
	}
	return fmt.Errorf("cannot verify Charles is closed: %w", err)
}
