package ionoscloud

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/libdns/libdns"
)

// These tests run against the real IONOS Cloud DNS API.
// Set these environment variables to run:
//
//	IONOS_DNS_TOKEN    — API token with DNS permissions
//	IONOS_TEST_ZONE    — Zone to use for testing (e.g., "example.com.")
//
// WARNING: Tests create and delete real DNS records.
// Use a dedicated test zone.

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("skipping: %s not set", key)
	}
	return v
}

func newTestProvider(t *testing.T) (*Provider, string) {
	t.Helper()
	token := envOrSkip(t, "IONOS_DNS_TOKEN")
	zone := envOrSkip(t, "IONOS_TEST_ZONE")
	if !strings.HasSuffix(zone, ".") {
		zone += "."
	}
	p := &Provider{
		APIToken:    token,
		APIEndpoint: defaultAPIBase,
		client:      &http.Client{Timeout: 30 * time.Second},
		logger:      zap.NewNop(),
		zones:       make(map[string]string),
	}
	return p, zone
}

func TestGetRecords(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	records, err := p.GetRecords(ctx, zone)
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}

	t.Logf("found %d records in zone %s", len(records), zone)
	for _, r := range records {
		rr := r.RR()
		t.Logf("  %s %s %s (TTL %s)", rr.Type, rr.Name, rr.Data, rr.TTL)
	}
}

func TestAppendAndDeleteRecords(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-test-" + time.Now().Format("150405")

	// Append
	created, err := p.AppendRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			TTL:  60 * time.Second,
			Text: "libdns-ionoscloud-test",
		},
	})
	if err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created record, got %d", len(created))
	}
	rr := created[0].RR()
	t.Logf("created: %s %s = %s", rr.Type, rr.Name, rr.Data)

	// Delete
	deleted, err := p.DeleteRecords(ctx, zone, created)
	if err != nil {
		t.Fatalf("DeleteRecords: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted record, got %d", len(deleted))
	}
	t.Logf("deleted record")
}

func TestSetRecords(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-set-" + time.Now().Format("150405")

	// Create via Set
	updated, err := p.SetRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			TTL:  60 * time.Second,
			Text: "version-1",
		},
	})
	if err != nil {
		t.Fatalf("SetRecords (create): %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 record, got %d", len(updated))
	}
	rr := updated[0].RR()
	t.Logf("set (create): %s = %s", rr.Name, rr.Data)

	// Update via Set (same name+type, different value)
	updated2, err := p.SetRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			TTL:  60 * time.Second,
			Text: "version-2",
		},
	})
	if err != nil {
		t.Fatalf("SetRecords (update): %v", err)
	}
	if len(updated2) != 1 {
		t.Fatalf("expected 1 record, got %d", len(updated2))
	}
	rr2 := updated2[0].RR()
	if !strings.Contains(rr2.Data, "version-2") {
		t.Fatalf("expected data containing 'version-2', got %q", rr2.Data)
	}
	t.Logf("set (update): %s = %s", rr2.Name, rr2.Data)

	// Cleanup
	_, err = p.DeleteRecords(ctx, zone, updated2)
	if err != nil {
		t.Fatalf("cleanup DeleteRecords: %v", err)
	}
	t.Log("cleanup done")
}

func TestDeleteByNameAndType(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-delbynametype-" + time.Now().Format("150405")

	// Create a record
	_, err := p.AppendRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			TTL:  60 * time.Second,
			Text: "delete-me",
		},
	})
	if err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}

	// Delete without ID (by name + type + value)
	deleted, err := p.DeleteRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			Text: "delete-me",
		},
	})
	if err != nil {
		t.Fatalf("DeleteRecords by name: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted, got %d", len(deleted))
	}
	t.Logf("deleted by name+type: %s", testName)
}

func TestRecordNameNotDoubled(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-namecheck-" + time.Now().Format("150405")

	// Create a record
	created, err := p.AppendRecords(ctx, zone, []libdns.Record{
		libdns.TXT{
			Name: testName,
			TTL:  60 * time.Second,
			Text: "namecheck",
		},
	})
	if err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}
	t.Cleanup(func() {
		p.DeleteRecords(ctx, zone, created)
	})

	// Verify the record is retrievable via GetRecords with the correct name
	records, err := p.GetRecords(ctx, zone)
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}

	zoneTrimmed := strings.TrimSuffix(zone, ".")
	for _, r := range records {
		rr := r.RR()
		if !strings.Contains(rr.Name, testName) {
			continue
		}
		t.Logf("record name: %q", rr.Name)

		// The name must NOT contain the zone (that would mean it was doubled)
		if strings.Contains(rr.Name, zoneTrimmed) {
			t.Fatalf("zone appears in record name — was doubled: %q", rr.Name)
		}

		// The name should be exactly the relative name we passed in
		if rr.Name != testName {
			t.Fatalf("expected name %q, got %q", testName, rr.Name)
		}
		return
	}
	t.Fatal("created record not found in GetRecords")
}

func TestCaddyfileUnmarshal(t *testing.T) {
	// Unit test — no API call needed
	tests := []struct {
		name     string
		input    string
		token    string
		endpoint string
	}{
		{
			name:  "inline token",
			input: `ionoscloud my-token-123`,
			token: "my-token-123",
		},
		{
			name:  "block syntax",
			input: "ionoscloud {\n  api_token my-token-456\n}",
			token: "my-token-456",
		},
		{
			name:     "block with endpoint",
			input:    "ionoscloud {\n  api_token my-token-789\n  api_endpoint https://custom.api.example.com\n}",
			token:    "my-token-789",
			endpoint: "https://custom.api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{}
			d := caddyfile.NewTestDispenser(tt.input)
			err := p.UnmarshalCaddyfile(d)
			if err != nil {
				t.Fatalf("UnmarshalCaddyfile: %v", err)
			}
			if p.APIToken != tt.token {
				t.Fatalf("expected token %q, got %q", tt.token, p.APIToken)
			}
			if tt.endpoint != "" && p.APIEndpoint != tt.endpoint {
				t.Fatalf("expected endpoint %q, got %q", tt.endpoint, p.APIEndpoint)
			}
		})
	}
}
