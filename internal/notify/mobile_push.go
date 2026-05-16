package notify

import (
	"context"
	"net/url"
	"strings"

	"github.com/ContinuumApp/continuum-plugin-notifications/internal/store"
)

func mobilePushProviders() []Provider {
	return []Provider{
		barkProvider(),
		chanifyProvider(),
		pushplusProvider(),
		serverChanProvider(),
		simplePushProvider(),
		pushdeerProvider(),
		pushsaferProvider(),
		pushmeProvider(),
		spugPushProvider(),
		spikeProvider(),
		noticaProvider(),
		popcornNotifyProvider(),
		techulusProvider(),
		qqProvider(),
		wxPusherProvider(),
	}
}

func barkProvider() Provider {
	return funcProvider{id: "bark", name: "Bark", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "device_key", Label: "Device key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := strings.TrimRight(val(t, "server"), "/") + "/" + val(t, "device_key") + "/" + url.PathEscape(m.Title) + "/" + url.PathEscape(m.Body)
		return postJSON(ctx, u, nil, map[string]any{})
	}}
}

func chanifyProvider() Provider {
	return funcProvider{id: "chanify", name: "Chanify", fields: []Field{{Key: "endpoint", Label: "Endpoint", Required: true}, {Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := strings.TrimRight(val(t, "endpoint"), "/") + "/v1/sender/" + val(t, "token")
		return postJSON(ctx, u, nil, map[string]any{"title": m.Title, "text": m.Body})
	}}
}

func pushplusProvider() Provider {
	return funcProvider{id: "pushplus", name: "Pushplus", fields: []Field{{Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://www.pushplus.plus/send", nil, map[string]any{"token": val(t, "token"), "title": m.Title, "content": m.Body})
	}}
}

func serverChanProvider() Provider {
	return funcProvider{id: "serverchan", name: "ServerChan", fields: []Field{{Key: "send_key", Label: "Send key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://sctapi.ftqq.com/"+val(t, "send_key")+".send", nil, url.Values{"title": {m.Title}, "desp": {m.Body}})
	}}
}

func simplePushProvider() Provider {
	return funcProvider{id: "simplepush", name: "SimplePush", fields: []Field{{Key: "key", Label: "Key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://api.simplepush.io/send", nil, url.Values{"key": {val(t, "key")}, "title": {m.Title}, "msg": {m.Body}})
	}}
}

func pushdeerProvider() Provider {
	return funcProvider{id: "pushdeer", name: "PushDeer", fields: []Field{{Key: "endpoint", Label: "Endpoint"}, {Key: "push_key", Label: "Push key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		endpoint := val(t, "endpoint")
		if endpoint == "" {
			endpoint = "https://api2.pushdeer.com"
		}
		return postForm(ctx, strings.TrimRight(endpoint, "/")+"/message/push", nil, url.Values{"pushkey": {val(t, "push_key")}, "text": {m.Title}, "desp": {m.Body}})
	}}
}

func pushsaferProvider() Provider {
	return funcProvider{id: "pushsafer", name: "Pushsafer", fields: []Field{{Key: "private_key", Label: "Private key", Secret: true, Required: true}, {Key: "device", Label: "Device"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://www.pushsafer.com/api", nil, url.Values{"k": {val(t, "private_key")}, "d": {val(t, "device")}, "t": {m.Title}, "m": {m.Body}})
	}}
}

func pushmeProvider() Provider {
	return funcProvider{id: "pushme", name: "PushMe", fields: []Field{{Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://push.i-i.me", nil, url.Values{"push_key": {val(t, "token")}, "title": {m.Title}, "content": {m.Body}})
	}}
}

func spugPushProvider() Provider {
	return funcProvider{id: "spugpush", name: "Spug Push", fields: []Field{{Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://push.spug.cc/send/"+val(t, "token"), nil, map[string]any{"title": m.Title, "content": m.Body})
	}}
}

func spikeProvider() Provider {
	return funcProvider{id: "spike", name: "Spike", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"title": m.Title, "text": m.Body})
	}}
}

func noticaProvider() Provider {
	return funcProvider{id: "notica", name: "Notica", fields: []Field{{Key: "token", Label: "Token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://notica.us/"+val(t, "token"), nil, url.Values{"title": {m.Title}, "message": {m.Body}})
	}}
}

func popcornNotifyProvider() Provider {
	return funcProvider{id: "popcorn_notify", name: "PopcornNotify", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://popcornnotify.com/notify", nil, url.Values{"api_key": {val(t, "api_key")}, "to": {val(t, "to")}, "subject": {m.Title}, "message": {m.Body}})
	}}
}

func techulusProvider() Provider {
	return funcProvider{id: "techuluspush", name: "Techulus Push", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://push.techulus.com/api/v1/notify/"+val(t, "api_key"), nil, map[string]any{"title": m.Title, "body": m.Body})
	}}
}

func qqProvider() Provider {
	return funcProvider{id: "qq", name: "QQ Push", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"title": m.Title, "content": m.Body})
	}}
}

func wxPusherProvider() Provider {
	return funcProvider{id: "wxpusher", name: "WxPusher", fields: []Field{{Key: "app_token", Label: "App token", Secret: true, Required: true}, {Key: "uids", Label: "UIDs"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://wxpusher.zjiecode.com/api/send/message", nil, map[string]any{"appToken": val(t, "app_token"), "content": m.Title + "\n\n" + m.Body, "summary": m.Title, "contentType": 1, "uids": splitCSV(val(t, "uids"))})
	}}
}

func splitCSV(raw string) []string {
	var out []string
	for _, v := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}
