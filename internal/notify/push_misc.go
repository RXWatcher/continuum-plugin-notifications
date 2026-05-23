package notify

import (
	"context"
	"net/url"
	"strings"

	"github.com/RXWatcher/silo-plugin-notifications/internal/store"
)

func remainingPushProviders() []Provider {
	return []Provider{
		pushyProvider(),
		pushedProvider(),
		pushjetProvider(),
		prowlProvider(),
		joinProvider(),
		notificoProvider(),
		lametricCloudProvider(),
		aprsProvider(),
		dapnetProvider(),
		dotProvider(),
	}
}

func pushyProvider() Provider {
	return funcProvider{id: "pushy", name: "Pushy", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "to", Label: "Device/topic", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.pushy.me/push?api_key="+url.QueryEscape(val(t, "api_key")), nil, map[string]any{"to": val(t, "to"), "data": map[string]any{"title": m.Title, "message": m.Body}, "notification": map[string]any{"title": m.Title, "body": m.Body}})
	}}
}

func pushedProvider() Provider {
	return funcProvider{id: "pushed", name: "Pushed", fields: []Field{{Key: "app_key", Label: "App key", Secret: true, Required: true}, {Key: "app_secret", Label: "App secret", Secret: true, Required: true}, {Key: "target", Label: "Target alias/user"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		v := url.Values{"app_key": {val(t, "app_key")}, "app_secret": {val(t, "app_secret")}, "content": {m.Body}, "content_type": {"text"}, "target_type": {"app"}}
		if target := val(t, "target"); target != "" {
			v.Set("target_alias", target)
		}
		return postForm(ctx, "https://api.pushed.co/1/push", nil, v)
	}}
}

func pushjetProvider() Provider {
	return funcProvider{id: "pushjet", name: "Pushjet", fields: []Field{{Key: "server", Label: "Server"}, {Key: "secret", Label: "Secret", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		server := val(t, "server")
		if server == "" {
			server = "https://api.pushjet.io"
		}
		return postForm(ctx, strings.TrimRight(server, "/")+"/message", nil, url.Values{"secret": {val(t, "secret")}, "title": {m.Title}, "message": {m.Body}})
	}}
}

func prowlProvider() Provider {
	return funcProvider{id: "prowl", name: "Prowl", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "priority", Label: "Priority"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		pri := val(t, "priority")
		if pri == "" {
			pri = "0"
		}
		return postForm(ctx, "https://api.prowlapp.com/publicapi/add", nil, url.Values{"apikey": {val(t, "api_key")}, "application": {"Silo"}, "event": {m.Title}, "description": {m.Body}, "priority": {pri}})
	}}
}

func joinProvider() Provider {
	return funcProvider{id: "join", name: "Join", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "device_id", Label: "Device/group"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		v := url.Values{"apikey": {val(t, "api_key")}, "title": {m.Title}, "text": {m.Body}}
		if d := val(t, "device_id"); d != "" {
			v.Set("deviceId", d)
		}
		return postForm(ctx, "https://joinjoaomgcd.appspot.com/_ah/api/messaging/v1/sendPush", nil, v)
	}}
}

func notificoProvider() Provider {
	return funcProvider{id: "notifico", name: "Notifico", fields: commonWebhookFields("webhook_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"payload": map[string]any{"commits": []any{map[string]any{"message": m.Title + "\n\n" + m.Body}}}})
	}}
}

func lametricCloudProvider() Provider {
	return funcProvider{id: "lametric_cloud", name: "LaMetric Cloud", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"frames": []any{map[string]any{"text": m.Title}, map[string]any{"text": m.Body}}})
	}}
}

func aprsProvider() Provider {
	return funcProvider{id: "aprs", name: "APRS-IS", fields: []Field{{Key: "server", Label: "Server host:port", Required: true}, {Key: "login", Label: "Login line", Secret: true, Required: true}, {Key: "packet", Label: "Packet prefix", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return tcpLine(ctx, val(t, "server"), val(t, "login")+"\n"+val(t, "packet")+":"+m.Title+" "+m.Body+"\n")
	}}
}

func dapnetProvider() Provider {
	return funcProvider{id: "dapnet", name: "DAPNET", fields: []Field{{Key: "endpoint", Label: "Endpoint", Required: true}, {Key: "authorization", Label: "Authorization", Secret: true}, {Key: "callsigns", Label: "Callsigns", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "endpoint"), map[string]string{"Authorization": val(t, "authorization")}, map[string]any{"text": m.Title + " " + m.Body, "callSignNames": splitCSV(val(t, "callsigns"))})
	}}
}

func dotProvider() Provider {
	return funcProvider{id: "dot", name: "Dot", fields: commonWebhookFields("webhook_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"text": m.Title + "\n" + m.Body})
	}}
}
