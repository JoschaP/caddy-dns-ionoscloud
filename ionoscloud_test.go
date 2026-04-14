package ionoscloud

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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
	return &Provider{APIToken: token}, zone
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
		t.Logf("  %s %s %s (TTL %s)", r.Type, r.Name, r.Value, r.TTL)
	}
}

func TestAppendAndDeleteRecords(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-test-" + time.Now().Format("150405")

	// Append
	created, err := p.AppendRecords(ctx, zone, []libdns.Record{
		{
			Type:  "TXT",
			Name:  testName,
			Value: "libdns-ionoscloud-test",
			TTL:   60 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created record, got %d", len(created))
	}
	t.Logf("created: %s %s = %s (ID: %s)", created[0].Type, created[0].Name, created[0].Value, created[0].ID)

	// Verify it exists
	records, err := p.GetRecords(ctx, zone)
	if err != nil {
		t.Fatalf("GetRecords after append: %v", err)
	}
	var found bool
	for _, r := range records {
		if r.ID == created[0].ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created record %s not found in zone", created[0].ID)
	}

	// Delete
	deleted, err := p.DeleteRecords(ctx, zone, created)
	if err != nil {
		t.Fatalf("DeleteRecords: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted record, got %d", len(deleted))
	}
	t.Logf("deleted: %s", deleted[0].ID)
}

func TestSetRecords(t *testing.T) {
	p, zone := newTestProvider(t)
	ctx := context.Background()

	testName := "_libdns-set-" + time.Now().Format("150405")

	// Create via Set
	updated, err := p.SetRecords(ctx, zone, []libdns.Record{
		{
			Type:  "TXT",
			Name:  testName,
			Value: "version-1",
			TTL:   60 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("SetRecords (create): %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected 1 record, got %d", len(updated))
	}
	t.Logf("set (create): %s = %s (ID: %s)", updated[0].Name, updated[0].Value, updated[0].ID)

	// Update via Set (same name+type, different value)
	updated2, err := p.SetRecords(ctx, zone, []libdns.Record{
		{
			Type:  "TXT",
			Name:  testName,
			Value: "version-2",
			TTL:   60 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("SetRecords (update): %v", err)
	}
	if len(updated2) != 1 {
		t.Fatalf("expected 1 record, got %d", len(updated2))
	}
	if updated2[0].Value != "version-2" {
		t.Fatalf("expected value 'version-2', got %q", updated2[0].Value)
	}
	t.Logf("set (update): %s = %s (ID: %s)", updated2[0].Name, updated2[0].Value, updated2[0].ID)

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
	created, err := p.AppendRecords(ctx, zone, []libdns.Record{
		{
			Type:  "TXT",
			Name:  testName,
			Value: "delete-me",
			TTL:   60 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("AppendRecords: %v", err)
	}

	// Delete without ID (by name + type + value)
	deleted, err := p.DeleteRecords(ctx, zone, []libdns.Record{
		{
			Type:  "TXT",
			Name:  testName,
			Value: "delete-me",
		},
	})
	if err != nil {
		t.Fatalf("DeleteRecords by name: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted, got %d", len(deleted))
	}
	t.Logf("deleted by name+type: %s (original ID: %s)", testName, created[0].ID)
}

func TestCaddyfileUnmarshal(t *testing.T) {
	// Unit test — no API call needed
	tests := []struct {
		name  string
		input string
		token string
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
		})
	}
}
