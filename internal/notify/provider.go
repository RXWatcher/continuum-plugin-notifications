package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/ContinuumApp/continuum-plugin-notifications/internal/store"
)

type Message struct {
	EventName   string         `json:"event_name"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	Payload     map[string]any `json:"payload"`
	Attachments []Attachment   `json:"attachments,omitempty"`
}

type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	DataBase64  string `json:"data_base64"`
}

type Provider interface {
	ID() string
	DisplayName() string
	Fields() []Field
	Capabilities() Capabilities
	Send(ctx context.Context, target store.Target, msg Message) error
}

type Capabilities struct {
	RecipientMode      string   `json:"recipient_mode"`
	DynamicRecipients  bool     `json:"dynamic_recipients"`
	Attachments        bool     `json:"attachments"`
	MaxAttachmentBytes int64    `json:"max_attachment_bytes,omitempty"`
	ContentTypes       []string `json:"content_types,omitempty"`
}

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Secret      bool     `json:"secret,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Control     string   `json:"control,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Default     string   `json:"default,omitempty"`
	Options     []Option `json:"options,omitempty"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Registry struct {
	providers map[string]Provider
	timeout   time.Duration
}

func NewRegistry(timeout time.Duration) *Registry {
	r := &Registry{providers: map[string]Provider{}, timeout: timeout}
	for _, group := range [][]Provider{
		coreProviders(),
		httpAPIProviders(),
		mobilePushProviders(),
		socialChatProviders(),
		smsGatewayProviders(),
		opsMediaProviders(),
		remainingPushProviders(),
		finalParityProviders(),
	} {
		for _, p := range group {
			if _, exists := r.providers[p.ID()]; !exists {
				r.providers[p.ID()] = p
			}
		}
	}
	return r
}

func (r *Registry) Provider(id string) (Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) Catalog() []map[string]any {
	out := make([]map[string]any, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, map[string]any{"id": p.ID(), "name": p.DisplayName(), "fields": p.Fields(), "capabilities": p.Capabilities(), "implemented": true})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["name"].(string) < out[j]["name"].(string)
	})
	return out
}

func (r *Registry) Send(ctx context.Context, target store.Target, msg Message) error {
	p, ok := r.Provider(target.Provider)
	if !ok {
		return fmt.Errorf("unknown provider %q", target.Provider)
	}
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	return p.Send(ctx, target, msg)
}

func val(t store.Target, key string) string { return strings.TrimSpace(t.Config[key]) }

func providerCapabilities(id string) Capabilities {
	switch id {
	case "smtp", "smtp2go", "sendgrid", "sendpulse", "postmark", "resend", "mailgun", "brevo", "sparkpost", "ses", "office365":
		return Capabilities{RecipientMode: "direct_addressable", DynamicRecipients: true, Attachments: id == "smtp", MaxAttachmentBytes: 25 * 1024 * 1024, ContentTypes: []string{"*/*"}}
	case "discord":
		return Capabilities{RecipientMode: "broadcast_channel", Attachments: true, MaxAttachmentBytes: 25 * 1024 * 1024, ContentTypes: []string{"*/*"}}
	case "slack", "teams", "msteams", "google_chat", "mattermost", "rocketchat", "webexteams", "guilded", "revolt", "flock", "ryver", "twist", "synology":
		return Capabilities{RecipientMode: "broadcast_channel"}
	}
	if p := strings.ToLower(id); strings.Contains(p, "sms") || p == "twilio" || p == "vonage" || p == "plivo" || p == "whatsapp" || p == "signal_api" || p == "line" || p == "viber" {
		return Capabilities{RecipientMode: "direct_addressable", DynamicRecipients: true}
	}
	return Capabilities{RecipientMode: "configured_target"}
}

func coreProviders() []Provider {
	return []Provider{
		funcProvider{id: "webhook", name: "Generic webhook", fields: []Field{{Key: "url", Label: "URL", Required: true}, {Key: "authorization", Label: "Authorization", Secret: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
			return postJSON(ctx, val(t, "url"), map[string]string{"Authorization": val(t, "authorization")}, m)
		}},
		funcProvider{id: "discord", name: "Discord webhook", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Secret: true, Required: true}, {Key: "username", Label: "Username"}}, send: func(ctx context.Context, t store.Target, m Message) error {
			body := map[string]any{"content": "**" + m.Title + "**\n" + m.Body}
			if u := val(t, "username"); u != "" {
				body["username"] = u
			}
			if len(m.Attachments) > 0 {
				return postDiscordWebhook(ctx, val(t, "webhook_url"), body, m.Attachments)
			}
			return postJSON(ctx, val(t, "webhook_url"), nil, body)
		}},
		jsonTextProvider("slack", "Slack incoming webhook", "webhook_url", "text"),
		jsonTextProvider("teams", "Microsoft Teams webhook", "webhook_url", "text"),
		funcProvider{id: "telegram", name: "Telegram bot", fields: []Field{{Key: "bot_token", Label: "Bot token", Secret: true, Required: true}, {Key: "chat_id", Label: "Chat ID", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
			u := "https://api.telegram.org/bot" + val(t, "bot_token") + "/sendMessage"
			return postJSON(ctx, u, nil, map[string]any{"chat_id": val(t, "chat_id"), "text": m.Title + "\n\n" + m.Body})
		}},
		funcProvider{id: "ntfy", name: "ntfy", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "topic", Label: "Topic", Required: true}, {Key: "token", Label: "Bearer token", Secret: true}}, send: ntfySend},
		funcProvider{id: "gotify", name: "Gotify", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "token", Label: "App token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
			u := strings.TrimRight(val(t, "server"), "/") + "/message?token=" + val(t, "token")
			return postJSON(ctx, u, nil, map[string]any{"title": m.Title, "message": m.Body, "priority": 5})
		}},
		funcProvider{id: "pushover", name: "Pushover", fields: []Field{{Key: "token", Label: "App token", Secret: true, Required: true}, {Key: "user", Label: "User/group key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
			return postJSON(ctx, "https://api.pushover.net/1/messages.json", nil, map[string]any{"token": val(t, "token"), "user": val(t, "user"), "title": m.Title, "message": m.Body})
		}},
		funcProvider{id: "smtp", name: "SMTP email", fields: []Field{{Key: "host", Label: "SMTP host", Required: true}, {Key: "port", Label: "Port", Required: true}, {Key: "tls_mode", Label: "TLS mode", Default: "starttls", Options: []Option{{Value: "starttls", Label: "STARTTLS"}, {Value: "tls", Label: "Implicit TLS"}, {Value: "none", Label: "None"}}}, {Key: "username", Label: "Username"}, {Key: "password", Label: "Password", Secret: true}, {Key: "from", Label: "From address", Required: true}, {Key: "to", Label: "Recipients", Required: true}}, send: smtpSend},
	}
}

func normalizeFields(fields []Field) []Field {
	out := make([]Field, len(fields))
	for i, f := range fields {
		out[i] = enrichField(f)
	}
	return out
}

func enrichField(f Field) Field {
	if f.Control == "" {
		f.Control = "text"
	}
	if f.Secret {
		f.Control = "password"
	}
	switch f.Key {
	case "url", "webhook_url", "bridge_url", "integration_url", "endpoint", "hec_url", "device_url":
		if f.Control == "text" {
			f.Control = "url"
		}
		if f.Placeholder == "" {
			f.Placeholder = "https://example.com/..."
		}
	case "server", "site":
		if f.Control == "text" {
			f.Control = "url"
		}
		if f.Placeholder == "" {
			f.Placeholder = "https://example.com"
		}
		f.Help = firstNonEmpty(f.Help, "Base URL without a trailing path unless the provider documentation says otherwise.")
	case "host":
		f.Placeholder = firstNonEmpty(f.Placeholder, "smtp.example.com")
	case "port":
		f.Control = "number"
		f.Placeholder = firstNonEmpty(f.Placeholder, "587")
		f.Help = firstNonEmpty(f.Help, "Common values: 587 for STARTTLS, 465 for implicit TLS, 25 for local relays.")
	case "tls_mode":
		f.Control = "options"
		f.Help = firstNonEmpty(f.Help, "Use STARTTLS for port 587, implicit TLS for port 465, or none for trusted local relays.")
	case "starttls":
		f.Control = "checkbox"
		f.Label = firstNonEmpty(f.Label, "Use STARTTLS")
	case "authorization":
		f.Placeholder = firstNonEmpty(f.Placeholder, "Bearer ..., Basic ..., or provider-specific value")
		f.Help = firstNonEmpty(f.Help, "Sent as the HTTP Authorization header when the endpoint requires one.")
	case "from":
		f.Placeholder = firstNonEmpty(f.Placeholder, "sender@example.com or sender ID")
	case "from_user":
		f.Placeholder = firstNonEmpty(f.Placeholder, "user@example.com or Microsoft Graph user ID")
	case "to", "recipients", "uids", "callsigns", "install_ids":
		f.Placeholder = firstNonEmpty(f.Placeholder, "one or more values, comma-separated where supported")
	case "email":
		f.Control = "email"
		f.Placeholder = firstNonEmpty(f.Placeholder, "bot@example.com")
	case "visibility":
		f.Control = "options"
		f.Default = firstNonEmpty(f.Default, "direct")
		f.Options = firstOptions(f.Options, []Option{{Value: "direct", Label: "Direct"}, {Value: "private", Label: "Private"}, {Value: "unlisted", Label: "Unlisted"}, {Value: "public", Label: "Public"}})
	case "severity":
		f.Control = "options"
		f.Default = firstNonEmpty(f.Default, "info")
		f.Options = firstOptions(f.Options, []Option{{Value: "info", Label: "Info"}, {Value: "warning", Label: "Warning"}, {Value: "error", Label: "Error"}, {Value: "critical", Label: "Critical"}})
	case "priority":
		f.Control = "options"
		f.Default = firstNonEmpty(f.Default, "0")
		f.Options = firstOptions(f.Options, []Option{{Value: "-2", Label: "Very low"}, {Value: "-1", Label: "Low"}, {Value: "0", Label: "Normal"}, {Value: "1", Label: "High"}, {Value: "2", Label: "Emergency"}})
	case "issue_type":
		f.Placeholder = firstNonEmpty(f.Placeholder, "Task")
		f.Help = firstNonEmpty(f.Help, "Defaults to Task when blank.")
	case "facility":
		f.Placeholder = firstNonEmpty(f.Placeholder, "local0")
	case "color":
		f.Control = "color"
		f.Default = firstNonEmpty(f.Default, "#00aaff")
	case "topic":
		f.Placeholder = firstNonEmpty(f.Placeholder, "alerts")
	case "client_id":
		f.Placeholder = firstNonEmpty(f.Placeholder, "continuum-notifications")
	}
	return f
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstOptions(current, fallback []Option) []Option {
	if len(current) > 0 {
		return current
	}
	return fallback
}

func postJSON(ctx context.Context, url string, headers map[string]string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snip)))
	}
	return nil
}

func postDiscordWebhook(ctx context.Context, endpoint string, payload map[string]any, attachments []Attachment) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := mw.WriteField("payload_json", string(payloadBytes)); err != nil {
		return err
	}
	fileIndex := 0
	for _, attachment := range attachments {
		if attachment.Filename == "" || attachment.DataBase64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(attachment.DataBase64)
		if err != nil {
			return fmt.Errorf("decode attachment %q: %w", attachment.Filename, err)
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[%d]"; filename=%q`, fileIndex, attachment.Filename))
		header.Set("Content-Type", contentType)
		part, err := mw.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err := part.Write(data); err != nil {
			return err
		}
		fileIndex++
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(snip)))
	}
	return nil
}

func ntfySend(ctx context.Context, t store.Target, m Message) error {
	server := strings.TrimRight(val(t, "server"), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/"+val(t, "topic"), strings.NewReader(m.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", mime.QEncoding.Encode("utf-8", m.Title))
	if tok := val(t, "token"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func smtpSend(ctx context.Context, t store.Target, m Message) error {
	host, port := val(t, "host"), val(t, "port")
	addr := host + ":" + port
	tlsMode := strings.ToLower(val(t, "tls_mode"))
	if tlsMode == "" && strings.EqualFold(val(t, "starttls"), "true") {
		tlsMode = "starttls"
	}
	if tlsMode == "" {
		tlsMode = "starttls"
	}

	var c *smtp.Client
	var err error
	if tlsMode == "tls" {
		conn, dialErr := tls.DialWithDialer(&net.Dialer{}, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if dialErr != nil {
			return dialErr
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return err
		}
	} else {
		c, err = smtp.Dial(addr)
	}
	if err != nil {
		return err
	}
	defer c.Close()
	if tlsMode == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if u := val(t, "username"); u != "" {
		if err := c.Auth(smtp.PlainAuth("", u, val(t, "password"), host)); err != nil {
			return err
		}
	}
	from, to := val(t, "from"), val(t, "to")
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range strings.Split(to, ",") {
		if err := c.Rcpt(strings.TrimSpace(rcpt)); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	err = writeEmail(w, from, to, m)
	if err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return c.Quit()
	}
}

func writeEmail(w io.Writer, from, to string, m Message) error {
	if len(m.Attachments) == 0 {
		_, err := fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", from, to, m.Title, m.Body)
		return err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")
	textHeader.Set("Content-Transfer-Encoding", "8bit")
	part, err := mw.CreatePart(textHeader)
	if err != nil {
		return err
	}
	if _, err := part.Write([]byte(m.Body)); err != nil {
		return err
	}
	for _, attachment := range m.Attachments {
		if attachment.Filename == "" || attachment.DataBase64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(attachment.DataBase64)
		if err != nil {
			return fmt.Errorf("decode attachment %q: %w", attachment.Filename, err)
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", contentType)
		header.Set("Content-Transfer-Encoding", "base64")
		header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, attachment.Filename))
		part, err := mw.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err := part.Write([]byte(wrapBase64(base64.StdEncoding.EncodeToString(data)))); err != nil {
			return err
		}
	}
	if err := mw.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n%s", from, to, m.Title, mw.Boundary(), body.String())
	return err
}

func wrapBase64(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76])
		b.WriteString("\r\n")
		s = s[76:]
	}
	b.WriteString(s)
	b.WriteString("\r\n")
	return b.String()
}
