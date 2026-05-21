package notify

import (
	"context"
	"encoding/base64"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func opsMediaProviders() []Provider {
	return []Provider{
		signl4Provider(),
		pagerTreeProvider(),
		splunkProvider(),
		jiraProvider(),
		parseProvider(),
		notificationAPIProvider(),
		kumulosProvider(),
		laMetricProvider(),
		streamlabsProvider(),
		synologyProvider(),
		nextcloudProvider(),
		kodiProvider("kodi", "Kodi"),
		kodiProvider("xbmc", "XBMC"),
		syslogProvider("syslog", "Syslog", "udp"),
		syslogProvider("rsyslog", "RSyslog", "tcp"),
	}
}

func signl4Provider() Provider {
	return funcProvider{id: "signl4", name: "SIGNL4", fields: commonWebhookFields("webhook_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"Title": m.Title, "Message": m.Body, "Source": "Continuum"})
	}}
}

func pagerTreeProvider() Provider {
	return funcProvider{id: "pagertree", name: "PagerTree", fields: commonWebhookFields("integration_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "integration_url"), nil, map[string]any{"title": m.Title, "description": m.Body, "event_type": "create"})
	}}
}

func splunkProvider() Provider {
	return funcProvider{id: "splunk", name: "Splunk HEC", fields: []Field{{Key: "hec_url", Label: "HEC URL", Required: true}, {Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "hec_url"), map[string]string{"Authorization": "Splunk " + val(t, "token")}, map[string]any{"event": map[string]any{"title": m.Title, "body": m.Body, "event": m.EventName, "payload": m.Payload}, "source": "continuum"})
	}}
}

func jiraProvider() Provider {
	return funcProvider{id: "jira", name: "Jira", fields: []Field{{Key: "site", Label: "Site", Required: true}, {Key: "email", Label: "Email", Required: true}, {Key: "api_token", Label: "API token", Secret: true, Required: true}, {Key: "project_key", Label: "Project key", Required: true}, {Key: "issue_type", Label: "Issue type"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		typ := val(t, "issue_type")
		if typ == "" {
			typ = "Task"
		}
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "email")+":"+val(t, "api_token")))
		body := map[string]any{"fields": map[string]any{"project": map[string]any{"key": val(t, "project_key")}, "summary": m.Title, "description": m.Body, "issuetype": map[string]any{"name": typ}}}
		return postJSON(ctx, strings.TrimRight(val(t, "site"), "/")+"/rest/api/2/issue", map[string]string{"Authorization": auth}, body)
	}}
}

func parseProvider() Provider {
	return funcProvider{id: "parseplatform", name: "Parse Platform", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "app_id", Label: "Application ID", Secret: true, Required: true}, {Key: "master_key", Label: "Master key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, strings.TrimRight(val(t, "server"), "/")+"/push", map[string]string{"X-Parse-Application-Id": val(t, "app_id"), "X-Parse-Master-Key": val(t, "master_key")}, map[string]any{"where": map[string]any{}, "data": map[string]any{"alert": m.Title + "\n" + m.Body}})
	}}
}

func notificationAPIProvider() Provider {
	return funcProvider{id: "notificationapi", name: "NotificationAPI", fields: []Field{{Key: "client_id", Label: "Client ID", Required: true}, {Key: "client_secret", Label: "Client secret", Secret: true, Required: true}, {Key: "user_id", Label: "User ID", Required: true}, {Key: "notification_id", Label: "Notification ID", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "client_id")+":"+val(t, "client_secret")))
		return postJSON(ctx, "https://api.notificationapi.com/"+val(t, "client_id")+"/sender", map[string]string{"Authorization": auth}, map[string]any{"notificationId": val(t, "notification_id"), "user": map[string]any{"id": val(t, "user_id")}, "mergeTags": map[string]any{"title": m.Title, "body": m.Body}})
	}}
}

func kumulosProvider() Provider {
	return funcProvider{id: "kumulos", name: "Kumulos", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "server_key", Label: "Server key", Secret: true, Required: true}, {Key: "install_ids", Label: "Install IDs"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "api_key")+":"+val(t, "server_key")))
		return postJSON(ctx, "https://push.kumulos.com/notifications", map[string]string{"Authorization": auth}, map[string]any{"title": m.Title, "message": m.Body, "installIds": splitCSV(val(t, "install_ids"))})
	}}
}

func laMetricProvider() Provider {
	return funcProvider{id: "lametric", name: "LaMetric local", fields: []Field{{Key: "device_url", Label: "Device URL", Required: true}, {Key: "api_key", Label: "API key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("dev:"+val(t, "api_key")))
		return postJSON(ctx, strings.TrimRight(val(t, "device_url"), "/")+"/api/v2/device/notifications", map[string]string{"Authorization": auth}, map[string]any{"priority": "info", "model": map[string]any{"frames": []any{map[string]any{"text": m.Title}, map[string]any{"text": m.Body}}}})
	}}
}

func streamlabsProvider() Provider {
	return funcProvider{id: "streamlabs", name: "Streamlabs", fields: []Field{{Key: "access_token", Label: "Access token", Secret: true, Required: true}, {Key: "identifier", Label: "Identifier"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://streamlabs.com/api/v1.0/alerts", nil, url.Values{"access_token": {val(t, "access_token")}, "type": {"donation"}, "message": {m.Title + "\n" + m.Body}, "identifier": {val(t, "identifier")}})
	}}
}

func synologyProvider() Provider {
	return funcProvider{id: "synology", name: "Synology Chat", fields: commonWebhookFields("webhook_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, val(t, "webhook_url"), nil, url.Values{"payload": {`{"text":` + quoteJSON(m.Title+"\n\n"+m.Body) + `}`}})
	}}
}

func nextcloudProvider() Provider {
	return funcProvider{id: "nextcloud", name: "Nextcloud notifications", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "username", Label: "Username", Required: true}, {Key: "password", Label: "Password/app token", Secret: true, Required: true}, {Key: "user", Label: "Target user", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "username")+":"+val(t, "password")))
		u := strings.TrimRight(val(t, "server"), "/") + "/ocs/v2.php/apps/notifications/api/v2/admin_notifications/" + val(t, "user")
		return postForm(ctx, u, map[string]string{"Authorization": auth, "OCS-APIRequest": "true"}, url.Values{"shortMessage": {m.Title}, "longMessage": {m.Body}})
	}}
}

func kodiProvider(id, name string) Provider {
	return funcProvider{id: id, name: name, fields: []Field{{Key: "url", Label: "JSON-RPC URL", Required: true}, {Key: "username", Label: "Username"}, {Key: "password", Label: "Password", Secret: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		headers := map[string]string{}
		if val(t, "username") != "" {
			headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "username")+":"+val(t, "password")))
		}
		return postJSON(ctx, val(t, "url"), headers, map[string]any{"jsonrpc": "2.0", "method": "GUI.ShowNotification", "params": map[string]any{"title": m.Title, "message": m.Body}, "id": "continuum"})
	}}
}

func syslogProvider(id, name, network string) Provider {
	return funcProvider{id: id, name: name, fields: []Field{{Key: "address", Label: "Address host:port", Required: true}, {Key: "facility", Label: "Facility"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		d := net.Dialer{Timeout: 5 * time.Second}
		c, err := d.DialContext(ctx, network, val(t, "address"))
		if err != nil {
			return err
		}
		defer c.Close()
		_, err = c.Write([]byte("<14>continuum-notifications: " + m.Title + " " + strings.ReplaceAll(m.Body, "\n", " ") + "\n"))
		return err
	}}
}
