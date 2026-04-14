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
)

const apiBase = "https://dns.de-fra.ionos.com"

func init() {
	caddy.RegisterModule(&Provider{})
}

// Provider implements the libdns interfaces for the IONOS Cloud DNS API.
type Provider struct {
	// APIToken is the IONOS Cloud API Bearer token with DNS permissions.
	APIToken string `json:"api_token,omitempty"`

	mu     sync.Mutex
	client *http.Client
}

// CaddyModule returns the Caddy module information.
func (*Provider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "dns.providers.ionoscloud",
		New: func() caddy.Module { return new(Provider) },
	}
}

// Provision validates the provider configuration.
func (p *Provider) Provision(ctx caddy.Context) error {
	p.APIToken = caddy.NewReplacer().ReplaceAll(p.APIToken, "")
	if p.APIToken == "" {
		return fmt.Errorf("ionoscloud: api_token is required")
	}
	return nil
}

// UnmarshalCaddyfile parses the Caddyfile DNS provider configuration.
//
//	ionoscloud [<api_token>]
//
//	ionoscloud {
//	    api_token <token>
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
			default:
				return d.Errf("unrecognized option: %s", d.Val())
			}
		}
	}
	return nil
}

// --- HTTP client ---

func (p *Provider) httpClient() *http.Client {
	if p.client == nil {
		p.client = &http.Client{Timeout: 30 * time.Second}
	}
	return p.client
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

	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("ionoscloud: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ionoscloud: %s %s: %d %s", method, path, resp.StatusCode, string(data))
	}
	return data, nil
}

// --- Zone lookup ---

type apiZone struct {
	ID         string `json:"id"`
	Properties struct {
		ZoneName string `json:"zoneName"`
	} `json:"properties"`
}

func (p *Provider) findZoneID(ctx context.Context, zone string) (string, error) {
	zone = strings.TrimSuffix(zone, ".")
	data, err := p.doAPI(ctx, http.MethodGet, "/zones", nil)
	if err != nil {
		return "", err
	}
	var resp struct{ Items []apiZone }
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	for _, z := range resp.Items {
		if strings.TrimSuffix(z.Properties.ZoneName, ".") == zone {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("ionoscloud: zone %q not found", zone)
}

// --- Record types ---

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

func toLibdns(r apiRecord, zone string) libdns.Record {
	zone = strings.TrimSuffix(zone, ".")
	name := strings.TrimSuffix(r.Properties.Name, "."+zone)
	name = strings.TrimSuffix(name, ".")
	return libdns.Record{
		ID:    r.ID,
		Type:  r.Properties.Type,
		Name:  name,
		Value: r.Properties.Content,
		TTL:   time.Duration(r.Properties.TTL) * time.Second,
	}
}

func toFQDN(name, zone string) string {
	zone = strings.TrimSuffix(zone, ".")
	name = strings.TrimSuffix(name, ".")
	if name == "" || name == "@" {
		return zone
	}
	return name + "." + zone
}

func ttlOrDefault(ttl time.Duration) int {
	s := int(ttl.Seconds())
	if s <= 0 {
		return 300
	}
	return s
}

// --- libdns interface ---

// GetRecords lists all records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

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
		records = append(records, toLibdns(r, zone))
	}
	return records, nil
}

// AppendRecords adds records to the zone.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	var created []libdns.Record
	for _, rec := range records {
		body := apiRecordCreate{}
		body.Properties.Name = toFQDN(rec.Name, zone)
		body.Properties.Type = rec.Type
		body.Properties.Content = rec.Value
		body.Properties.TTL = ttlOrDefault(rec.TTL)
		body.Properties.Enabled = true
		data, err := p.doAPI(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/records", zoneID), body)
		if err != nil {
			return created, err
		}
		var result apiRecord
		if err := json.Unmarshal(data, &result); err != nil {
			return created, err
		}
		created = append(created, toLibdns(result, zone))
	}
	return created, nil
}

// SetRecords sets records in the zone, replacing existing ones with matching name+type.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	// Fetch existing for matching
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
		fqdn := toFQDN(rec.Name, zone)
		body := apiRecordCreate{}
		body.Properties.Name = fqdn
		body.Properties.Type = rec.Type
		body.Properties.Content = rec.Value
		body.Properties.TTL = ttlOrDefault(rec.TTL)
		body.Properties.Enabled = true

		// Find existing by name+type
		var id string
		for _, e := range existing.Items {
			if e.Properties.Name == fqdn && e.Properties.Type == rec.Type {
				id = e.ID
				break
			}
		}

		var respData []byte
		if id != "" {
			respData, err = p.doAPI(ctx, http.MethodPut, fmt.Sprintf("/zones/%s/records/%s", zoneID, id), body)
		} else {
			respData, err = p.doAPI(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/records", zoneID), body)
		}
		if err != nil {
			return updated, err
		}

		var result apiRecord
		if err := json.Unmarshal(respData, &result); err != nil {
			return updated, err
		}
		updated = append(updated, toLibdns(result, zone))
	}
	return updated, nil
}

// DeleteRecords deletes records from the zone.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	zoneID, err := p.findZoneID(ctx, zone)
	if err != nil {
		return nil, err
	}

	// Fetch existing if any records lack IDs
	var existingRecords []apiRecord
	for _, r := range records {
		if r.ID == "" {
			data, err := p.doAPI(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/records", zoneID), nil)
			if err != nil {
				return nil, err
			}
			var resp struct{ Items []apiRecord }
			json.Unmarshal(data, &resp)
			existingRecords = resp.Items
			break
		}
	}

	var deleted []libdns.Record
	for _, rec := range records {
		id := rec.ID
		if id == "" {
			fqdn := toFQDN(rec.Name, zone)
			for _, e := range existingRecords {
				if e.Properties.Name == fqdn && e.Properties.Type == rec.Type && e.Properties.Content == rec.Value {
					id = e.ID
					break
				}
			}
			if id == "" {
				continue
			}
		}
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
