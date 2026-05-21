package notify

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func socialChatProviders() []Provider {
	return []Provider{
		mastodonProvider(),
		misskeyProvider(),
		blueskyProvider(),
		lineProvider(),
		viberProvider(),
		whatsappProvider(),
		signalProvider(),
		zulipProvider(),
		nextcloudTalkProvider(),
		flockProvider(),
		ryverProvider(),
		twistProvider(),
		redditWebhookProvider(),
	}
}

func mastodonProvider() Provider {
	return funcProvider{id: "mastodon", name: "Mastodon", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "access_token", Label: "Access token", Secret: true, Required: true}, {Key: "visibility", Label: "Visibility"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		visibility := val(t, "visibility")
		if visibility == "" {
			visibility = "direct"
		}
		return postForm(ctx, strings.TrimRight(val(t, "server"), "/")+"/api/v1/statuses", map[string]string{"Authorization": "Bearer " + val(t, "access_token")}, url.Values{"status": {m.Title + "\n\n" + m.Body}, "visibility": {visibility}})
	}}
}

func misskeyProvider() Provider {
	return funcProvider{id: "misskey", name: "Misskey", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "token", Label: "API token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, strings.TrimRight(val(t, "server"), "/")+"/api/notes/create", nil, map[string]any{"i": val(t, "token"), "text": m.Title + "\n\n" + m.Body})
	}}
}

func blueskyProvider() Provider {
	return funcProvider{id: "bluesky", name: "Bluesky", fields: []Field{{Key: "handle", Label: "Handle", Required: true}, {Key: "app_password", Label: "App password", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		var sess struct {
			AccessJWT string `json:"accessJwt"`
			Did       string `json:"did"`
		}
		if err := postJSONDecode(ctx, "https://bsky.social/xrpc/com.atproto.server.createSession", nil, map[string]any{"identifier": val(t, "handle"), "password": val(t, "app_password")}, &sess); err != nil {
			return err
		}
		return postJSON(ctx, "https://bsky.social/xrpc/com.atproto.repo.createRecord", map[string]string{"Authorization": "Bearer " + sess.AccessJWT}, map[string]any{
			"repo":       sess.Did,
			"collection": "app.bsky.feed.post",
			"record": map[string]any{
				"$type":     "app.bsky.feed.post",
				"text":      m.Title + "\n\n" + m.Body,
				"createdAt": nowRFC3339(),
			},
		})
	}}
}

func lineProvider() Provider {
	return funcProvider{id: "line", name: "LINE", fields: []Field{{Key: "channel_token", Label: "Channel access token", Secret: true, Required: true}, {Key: "to", Label: "User/group ID", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.line.me/v2/bot/message/push", map[string]string{"Authorization": "Bearer " + val(t, "channel_token")}, map[string]any{"to": val(t, "to"), "messages": []any{map[string]any{"type": "text", "text": m.Title + "\n\n" + m.Body}}})
	}}
}

func viberProvider() Provider {
	return funcProvider{id: "viber", name: "Viber", fields: []Field{{Key: "token", Label: "Bot token", Secret: true, Required: true}, {Key: "receiver", Label: "Receiver", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://chatapi.viber.com/pa/send_message", map[string]string{"X-Viber-Auth-Token": val(t, "token")}, map[string]any{"receiver": val(t, "receiver"), "type": "text", "text": m.Title + "\n\n" + m.Body})
	}}
}

func whatsappProvider() Provider {
	return funcProvider{id: "whatsapp", name: "WhatsApp Cloud API", fields: []Field{{Key: "token", Label: "Access token", Secret: true, Required: true}, {Key: "phone_number_id", Label: "Phone number ID", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := "https://graph.facebook.com/v20.0/" + val(t, "phone_number_id") + "/messages"
		return postJSON(ctx, u, map[string]string{"Authorization": "Bearer " + val(t, "token")}, map[string]any{"messaging_product": "whatsapp", "to": val(t, "to"), "type": "text", "text": map[string]any{"body": m.Title + "\n\n" + m.Body}})
	}}
}

func signalProvider() Provider {
	return funcProvider{id: "signal_api", name: "Signal API", fields: []Field{{Key: "server", Label: "Signal REST API server", Required: true}, {Key: "from", Label: "From", Required: true}, {Key: "to", Label: "Recipients", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, strings.TrimRight(val(t, "server"), "/")+"/v2/send", nil, map[string]any{"number": val(t, "from"), "recipients": splitCSV(val(t, "to")), "message": m.Title + "\n\n" + m.Body})
	}}
}

func zulipProvider() Provider {
	return funcProvider{id: "zulip", name: "Zulip", fields: []Field{{Key: "site", Label: "Site", Required: true}, {Key: "email", Label: "Bot email", Required: true}, {Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "to", Label: "Stream/user", Required: true}, {Key: "topic", Label: "Topic"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "email")+":"+val(t, "api_key")))
		msgType := "stream"
		if strings.Contains(val(t, "to"), "@") {
			msgType = "private"
		}
		return postForm(ctx, strings.TrimRight(val(t, "site"), "/")+"/api/v1/messages", map[string]string{"Authorization": auth}, url.Values{"type": {msgType}, "to": {val(t, "to")}, "topic": {val(t, "topic")}, "content": {m.Title + "\n\n" + m.Body}})
	}}
}

func nextcloudTalkProvider() Provider {
	return funcProvider{id: "nextcloudtalk", name: "Nextcloud Talk", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "room", Label: "Room token", Required: true}, {Key: "username", Label: "Username", Required: true}, {Key: "password", Label: "Password/app token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := strings.TrimRight(val(t, "server"), "/") + "/ocs/v2.php/apps/spreed/api/v1/chat/" + val(t, "room")
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "username")+":"+val(t, "password")))
		return postForm(ctx, u, map[string]string{"Authorization": auth, "OCS-APIRequest": "true"}, url.Values{"message": {m.Title + "\n\n" + m.Body}})
	}}
}

func flockProvider() Provider {
	return jsonTextProvider("flock", "Flock webhook", "webhook_url", "text")
}
func ryverProvider() Provider {
	return jsonTextProvider("ryver", "Ryver webhook", "webhook_url", "body")
}
func twistProvider() Provider {
	return jsonTextProvider("twist", "Twist webhook", "webhook_url", "content")
}
func redditWebhookProvider() Provider {
	return funcProvider{id: "reddit", name: "Reddit webhook bridge", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"title": m.Title, "body": m.Body})
	}}
}
