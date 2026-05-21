package notify

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func TestCatalogParitySurface(t *testing.T) {
	catalog := NewRegistry(5 * time.Second).Catalog()
	implemented := 0
	var pending []any
	for _, row := range catalog {
		if row["implemented"] != false {
			implemented++
		} else {
			pending = append(pending, row["id"])
		}
	}
	if len(catalog) < 133 {
		t.Fatalf("catalog has %d providers, want at least Apprise parity surface of 133", len(catalog))
	}
	if implemented < 100 {
		t.Fatalf("catalog has %d implemented providers, want at least 100", implemented)
	}
	t.Logf("provider catalog: total=%d implemented=%d pending=%d pending_ids=%v", len(catalog), implemented, len(catalog)-implemented, pending)
}

func TestDiscordWebhookAttachments(t *testing.T) {
	var contentType string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	registry := NewRegistry(time.Second)
	err := registry.Send(context.Background(), storeTarget("discord", map[string]string{"webhook_url": srv.URL}), Message{
		Title: "Book ready",
		Body:  "Attached.",
		Attachments: []Attachment{{
			Filename:    "book.epub",
			ContentType: "application/epub+zip",
			DataBase64:  base64.StdEncoding.EncodeToString([]byte("epub bytes")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data;") {
		t.Fatalf("content type = %q, want multipart/form-data", contentType)
	}
	for _, want := range []string{"payload_json", "Book ready", "files[0]", "book.epub", "application/epub+zip"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("multipart body missing %q: %s", want, string(body))
		}
	}
}

func storeTarget(provider string, config map[string]string) store.Target {
	return store.Target{ID: "target-1", Name: provider, Provider: provider, Enabled: true, Config: config}
}

func TestProviderConfigurationMetadata(t *testing.T) {
	registry := NewRegistry(5 * time.Second)
	provider, ok := registry.Provider("smtp")
	if !ok {
		t.Fatal("smtp provider missing")
	}

	fields := map[string]Field{}
	for _, field := range provider.Fields() {
		fields[field.Key] = field
	}

	if got := fields["tls_mode"].Control; got != "options" {
		t.Fatalf("smtp tls_mode control = %q, want options", got)
	}
	if len(fields["tls_mode"].Options) != 3 {
		t.Fatalf("smtp tls_mode option count = %d, want 3", len(fields["tls_mode"].Options))
	}
	if got := fields["port"].Control; got != "number" {
		t.Fatalf("smtp port control = %q, want number", got)
	}
	if fields["to"].Placeholder == "" {
		t.Fatal("smtp recipients field should include placeholder guidance")
	}

	provider, ok = registry.Provider("pagerduty")
	if !ok {
		t.Fatal("pagerduty provider missing")
	}
	for _, field := range provider.Fields() {
		if field.Key == "severity" && (field.Control != "options" || len(field.Options) == 0) {
			t.Fatalf("pagerduty severity metadata incomplete: %+v", field)
		}
	}

	provider, ok = registry.Provider("ses")
	if !ok {
		t.Fatal("ses provider missing")
	}
	fields = map[string]Field{}
	for _, field := range provider.Fields() {
		fields[field.Key] = field
	}
	for _, key := range []string{"endpoint", "authorization", "from", "to"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("ses provider missing %q field", key)
		}
	}
}

func TestProviderCatalogLabelsBridgeBackedProviders(t *testing.T) {
	catalog := NewRegistry(5 * time.Second).Catalog()
	byID := map[string]map[string]any{}
	for _, row := range catalog {
		byID[row["id"].(string)] = row
	}

	for _, id := range []string{"dbus", "smpp", "blink1", "reddit"} {
		row := byID[id]
		if row == nil {
			t.Fatalf("provider %q missing", id)
		}
		if got := row["delivery_kind"]; got != "bridge" {
			t.Fatalf("%s delivery_kind = %v, want bridge", id, got)
		}
		if got := row["implementation_note"]; got == "" {
			t.Fatalf("%s should include implementation_note", id)
		}
	}

	if got := byID["smtp"]["delivery_kind"]; got != "native" {
		t.Fatalf("smtp delivery_kind = %v, want native", got)
	}
}
