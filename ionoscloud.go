// Package ionoscloud implements a Caddy DNS provider for the IONOS Cloud DNS API.
//
// This uses the IONOS Cloud DNS API at dns.de-fra.ionos.com (NOT the IONOS
// Hosting DNS API at api.hosting.ionos.com — those are different products).
//
// It implements the libdns interfaces for DNS record management and registers
// as a Caddy DNS module for automatic TLS certificate provisioning via DNS-01.
//
// Caddyfile usage:
//
//	tls {
//	    dns ionoscloud {
//	        api_token {$IONOS_DNS_TOKEN}
//	    }
//	}
package ionoscloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/libdns/libdns"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const defaultAPIBase = "https://dns.de-fra.ionos.com"

func init() {
	caddy.RegisterModule(&Provider{})
}

// Provider implements the libdns interfaces for the IONOS Cloud DNS API.
type Provider struct {
	// APIToken is the IONOS Cloud API Bearer token with DNS permissions.
	APIToken string `json:"api_token,omitempty"`

	// APIEndpoint overrides the default IONOS Cloud DNS API URL.
	// Leave empty to use the default (dns.de-fra.ionos.com).
	APIEndpoint string `json:"api_endpoint,omitempty"`

	logger      *zap.Logger
	client      *http.Client
	zones       map[string]string // zone name → zone ID cache
	zonesCached time.Time         // when the cache was last populated
	zonesMu     sync.RWMutex
	zoneFlight  singleflight.Group // coalesces concurrent zone lookups
}

func (*Provider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dns.providers.ionoscloud",
		New: func() caddy.Module { return new(Provider) },
	}
}

// Provision sets up the HTTP client, logger, and validates the API token.
func (p *Provider) Provision(ctx caddy.Context) error {
	p.logger = ctx.Logger()
	p.APIToken = caddy.NewReplacer().ReplaceAll(p.APIToken, "")
	if p.APIToken == "" {
		return fmt.Errorf("ionoscloud: api_token is required")
	}
	if p.APIEndpoint == "" {
		p.APIEndpoint = defaultAPIBase
	}
	p.client = &http.Client{Timeout: 30 * time.Second}
	p.zones = make(map[string]string)
	return nil
}

// UnmarshalCaddyfile parses the Caddyfile DNS provider configuration.
//
//	ionoscloud [<api_token>]
//
//	ionoscloud {
//	    api_token    <token>
//	    api_endpoint <url>
//	}
func (p *Provider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if d.NextArg() {
			p.APIToken = d.Val()
		}
		for d.NextBlock(0) {
			switch d.Val() {
			case "api_token":
				if !d.NextArg() {
					return d.ArgErr()
				}
				p.APIToken = d.Val()
			case "api_endpoint":
				if !d.NextArg() {
					return d.ArgErr()
				}
				p.APIEndpoint = d.Val()
			default:
				return d.Errf("unrecognized option: %s", d.Val())
			}
		}
	}
	return nil
}

// apiError represents a structured error response from the IONOS Cloud DNS API.
type apiError struct {
	HTTPStatus int `json:"httpStatus"`
	Messages   []struct {
		ErrorCode string `json:"errorCode"`
		Message   string `json:"message"`
	} `json:"messages"`
}

func (p *Provider) doAPI(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("ionoscloud: marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.APIEndpoint+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ionoscloud: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Parse structured error if possible, never log raw body (may contain tokens)
		var apiErr apiError
		if json.Unmarshal(data, &apiErr) == nil && len(apiErr.Messages) > 0 {
			msgs := make([]string, len(apiErr.Messages))
			for i, m := range apiErr.Messages {
				msgs[i] = m.Message
			}
			return nil, fmt.Errorf("ionoscloud: %s %s: %d %s", method, path, resp.StatusCode, strings.Join(msgs, "; "))
		}
		// Fallback: truncate raw body to avoid leaking sensitive data
		rawBody := string(data)
		if len(rawBody) > 200 {
			rawBody = rawBody[:200] + "..."
		}
		return nil, fmt.Errorf("ionoscloud: %s %s: %d %s", method, path, resp.StatusCode, rawBody)
	}
	return data, nil
}

type apiZone struct {
	ID         string `json:"id"`
	Properties struct {
		ZoneName string `json:"zoneName"`
	} `json:"properties"`
}

const zoneCacheTTL = 1 * time.Hour

func (p *Provider) findZoneID(ctx context.Context, zone string) (string, error) {
	zone = strings.TrimSuffix(zone, ".")

	// Check cache (valid for zoneCacheTTL)
	p.zonesMu.RLock()
	if id, ok := p.zones[zone]; ok && time.Since(p.zonesCached) < zoneCacheTTL {
		p.zonesMu.RUnlock()
		return id, nil
	}
	p.zonesMu.RUnlock()

	// Coalesce concurrent cache misses into a single API call
	_, err, _ := p.zoneFlight.Do("zones", func() (interface{}, error) {
		p.logger.Debug("fetching zones from API")

		data, err := p.doAPI(ctx, http.MethodGet, "/zones", nil)
		if err != nil {
			return nil, err
		}
		var resp struct{ Items []apiZone }
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}

		p.zonesMu.Lock()
		p.zones = make(map[string]string, len(resp.Items))
		for _, z := range resp.Items {
			name := strings.TrimSuffix(z.Properties.ZoneName, ".")
			p.zones[name] = z.ID
		}
		p.zonesCached = time.Now()
		p.zonesMu.Unlock()
		return nil, nil
	})
	if err != nil {
		return "", err
	}

	p.zonesMu.RLock()
	id, ok := p.zones[zone]
	p.zonesMu.RUnlock()
	if !ok {
		return "", fmt.Errorf("ionoscloud: zone %q not found", zone)
	}
	return id, nil
}

type apiRecord struct {
	ID         string `json:"id,omitempty"`
	Properties struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Content  string `json:"content"`
		TTL      int    `json:"ttl"`
		Enabled  bool   `json:"enabled"`
		Priority int    `json:"priority"`
	} `json:"properties"`
}

type apiRecordCreate struct {
	Properties struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
		TTL     int    `json:"ttl"`
		Enabled bool   `json:"enabled"`
	} `json:"properties"`
}

func toLibdns(r apiRecord) libdns.Record {
	rr := libdns.RR{
		Name: r.Properties.Name,
		Type: r.Properties.Type,
		TTL:  time.Duration(r.Properties.TTL) * time.Second,
		Data: r.Properties.Content,
	}
	rec, err := rr.Parse()
	if err != nil {
		return rr
	}
	return rec
}

// toRelative returns the relative record name for the IONOS API.
// The IONOS Cloud DNS API expects relative names — it appends the zone automatically.
func toRelative(name string) string {
	name = strings.TrimSuffix(name, ".")
	if name == "@" {
		return ""
	}
	return name
}

func normalizeContent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}

func ttlOrDefault(ttl time.Duration) int {
	s := int(ttl.Seconds())
	if s <= 0 {
		return 300
	}
	return s
}

func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	data, err := p.doAPI(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/records", zoneID), nil)
	if err != nil {
		return nil, err
	}

	var resp struct{ Items []apiRecord }
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	records := make([]libdns.Record, 0, len(resp.Items))
	for _, r := range resp.Items {
		records = append(records, toLibdns(r))
	}

	p.logger.Debug("fetched records", zap.String("zone", zone), zap.Int("count", len(records)))
	return records, nil
}

func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	var created []libdns.Record
	for _, rec := range records {
		rr := rec.RR()
		body := apiRecordCreate{}
		body.Properties.Name = toRelative(rr.Name)
		body.Properties.Type = rr.Type
		body.Properties.Content = normalizeContent(rr.Data)
		body.Properties.TTL = ttlOrDefault(rr.TTL)
		body.Properties.Enabled = true

		p.logger.Debug("creating record", zap.String("zone", zone), zap.String("name", body.Properties.Name), zap.String("type", rr.Type))

		data, err := p.doAPI(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/records", zoneID), body)
		if err != nil {
			return created, err
		}
		var result apiRecord
		if err := json.Unmarshal(data, &result); err != nil {
			return created, err
		}
		created = append(created, toLibdns(result))
	}
	return created, nil
}

func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	data, err := p.doAPI(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/records", zoneID), nil)
	if err != nil {
		return nil, err
	}
	var existing struct{ Items []apiRecord }
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, err
	}

	var updated []libdns.Record
	for _, rec := range records {
		rr := rec.RR()
		relName := toRelative(rr.Name)
		body := apiRecordCreate{}
		body.Properties.Name = relName
		body.Properties.Type = rr.Type
		body.Properties.Content = normalizeContent(rr.Data)
		body.Properties.TTL = ttlOrDefault(rr.TTL)
		body.Properties.Enabled = true

		// Match by name+type+content first, then fall back to name+type only
		content := normalizeContent(rr.Data)
		var id string
		for _, e := range existing.Items {
			if e.Properties.Name == relName && e.Properties.Type == rr.Type && normalizeContent(e.Properties.Content) == content {
				id = e.ID
				break
			}
		}
		if id == "" {
			for _, e := range existing.Items {
				if e.Properties.Name == relName && e.Properties.Type == rr.Type {
					id = e.ID
					break
				}
			}
		}

		var respData []byte
		if id != "" {
			p.logger.Debug("updating record", zap.String("zone", zone), zap.String("name", relName), zap.String("type", rr.Type), zap.String("id", id))
			respData, err = p.doAPI(ctx, http.MethodPut, fmt.Sprintf("/zones/%s/records/%s", zoneID, id), body)
		} else {
			p.logger.Debug("creating record via set", zap.String("zone", zone), zap.String("name", relName), zap.String("type", rr.Type))
			respData, err = p.doAPI(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/records", zoneID), body)
		}
		if err != nil {
			return updated, err
		}

		var result apiRecord
		if err := json.Unmarshal(respData, &result); err != nil {
			return updated, err
		}
		updated = append(updated, toLibdns(result))
	}
	return updated, nil
}

func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	data, err := p.doAPI(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/records", zoneID), nil)
	if err != nil {
		return nil, err
	}
	var existing struct{ Items []apiRecord }
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil, fmt.Errorf("ionoscloud: unmarshal zone records: %w", err)
	}

	var deleted []libdns.Record
	for _, rec := range records {
		rr := rec.RR()
		relName := toRelative(rr.Name)
		content := normalizeContent(rr.Data)

		var id string
		for _, e := range existing.Items {
			if e.Properties.Name == relName && e.Properties.Type == rr.Type {
				if content == "" || normalizeContent(e.Properties.Content) == content {
					id = e.ID
					break
				}
			}
		}
		if id == "" {
			continue
		}

		p.logger.Debug("deleting record", zap.String("zone", zone), zap.String("name", relName), zap.String("type", rr.Type), zap.String("id", id))

		_, err := p.doAPI(ctx, http.MethodDelete, fmt.Sprintf("/zones/%s/records/%s", zoneID, id), nil)
		if err != nil {
			return deleted, err
		}
		deleted = append(deleted, rec)
	}
	return deleted, nil
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
	_ caddy.Provisioner     = (*Provider)(nil)
	_ caddyfile.Unmarshaler = (*Provider)(nil)
)
