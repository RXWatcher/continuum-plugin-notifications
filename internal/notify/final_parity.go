package notify

import (
	"context"
	"encoding/base64"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func finalParityProviders() []Provider {
	return []Provider{
		snsProvider(),
		blink1Provider(),
		bulkVSProvider(),
		desktopBridgeProvider("dbus", "DBus notification bridge"),
		enigma2Provider(),
		evolutionProvider(),
		fluxerProvider(),
		desktopBridgeProvider("glib", "GLib notification bridge"),
		desktopBridgeProvider("gnome", "GNOME notification bridge"),
		growlProvider(),
		mqttProvider(),
		office365Provider(),
		smppBridgeProvider(),
		smtp2goProvider(),
		sendpulseProvider(),
		threemaProvider(),
		twitterProvider(),
		wechatProvider(),
		desktopBridgeProvider("windows", "Windows notification bridge"),
		desktopBridgeProvider("macosx", "macOS notification bridge"),
	}
}

func snsProvider() Provider {
	return funcProvider{id: "sns", name: "AWS SNS signed endpoint", fields: []Field{{Key: "endpoint", Label: "Signed SNS endpoint", Required: true}, {Key: "authorization", Label: "Authorization", Secret: true}, {Key: "target", Label: "Phone/topic ARN", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, val(t, "endpoint"), map[string]string{"Authorization": val(t, "authorization")}, url.Values{"Action": {"Publish"}, "Message": {m.Title + "\n" + m.Body}, "TargetArn": {val(t, "target")}, "PhoneNumber": {val(t, "target")}})
	}}
}

func blink1Provider() Provider {
	return funcProvider{id: "blink1", name: "Blink(1) HTTP bridge", fields: []Field{{Key: "bridge_url", Label: "Bridge URL", Required: true}, {Key: "color", Label: "Color"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		color := val(t, "color")
		if color == "" {
			color = "#00aaff"
		}
		return postJSON(ctx, val(t, "bridge_url"), nil, map[string]any{"color": color, "title": m.Title, "body": m.Body})
	}}
}

func bulkVSProvider() Provider {
	return funcProvider{id: "bulkvs", name: "BulkVS SMS", fields: smsFields("Username", "Password"), send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "account")+":"+val(t, "token")))
		return postJSON(ctx, "https://portal.bulkvs.com/sendSMS", map[string]string{"Authorization": auth}, map[string]any{"From": val(t, "from"), "To": val(t, "to"), "Message": m.Title + "\n" + m.Body})
	}}
}

func desktopBridgeProvider(id, name string) Provider {
	return funcProvider{id: id, name: name, fields: []Field{{Key: "bridge_url", Label: "Desktop bridge URL", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "bridge_url"), nil, map[string]any{"title": m.Title, "body": m.Body, "event": m.EventName})
	}}
}

func enigma2Provider() Provider {
	return funcProvider{id: "enigma2", name: "Enigma2", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "username", Label: "Username"}, {Key: "password", Label: "Password", Secret: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		headers := map[string]string{}
		if val(t, "username") != "" {
			headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "username")+":"+val(t, "password")))
		}
		u := strings.TrimRight(val(t, "server"), "/") + "/web/message?type=1&timeout=10&text=" + url.QueryEscape(m.Title+"\n"+m.Body)
		return postRaw(ctx, u, "text/plain", headers, "")
	}}
}

func evolutionProvider() Provider {
	return funcProvider{id: "evolution", name: "Evolution API", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "instance", Label: "Instance", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := strings.TrimRight(val(t, "server"), "/") + "/message/sendText/" + val(t, "instance")
		return postJSON(ctx, u, map[string]string{"apikey": val(t, "api_key")}, map[string]any{"number": val(t, "to"), "text": m.Title + "\n\n" + m.Body})
	}}
}

func fluxerProvider() Provider {
	return funcProvider{id: "fluxer", name: "Fluxer", fields: commonWebhookFields("webhook_url"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), nil, map[string]any{"title": m.Title, "message": m.Body})
	}}
}

func growlProvider() Provider {
	return funcProvider{id: "growl", name: "Growl GNTP", fields: []Field{{Key: "address", Label: "Address host:port", Required: true}, {Key: "password", Label: "Password", Secret: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		line := "GNTP/1.0 NOTIFY NONE\r\nApplication-Name: Continuum\r\nNotification-Name: Continuum\r\nNotification-Title: " + m.Title + "\r\nNotification-Text: " + strings.ReplaceAll(m.Body, "\n", " ") + "\r\n\r\n"
		return tcpLine(ctx, val(t, "address"), line)
	}}
}

func mqttProvider() Provider {
	return funcProvider{id: "mqtt", name: "MQTT", fields: []Field{{Key: "address", Label: "Broker host:port", Required: true}, {Key: "topic", Label: "Topic", Required: true}, {Key: "client_id", Label: "Client ID"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		clientID := val(t, "client_id")
		if clientID == "" {
			clientID = "continuum-notifications"
		}
		return mqttPublish(ctx, val(t, "address"), clientID, val(t, "topic"), m.Title+"\n"+m.Body)
	}}
}

func office365Provider() Provider {
	return funcProvider{id: "office365", name: "Office 365 Graph mail", fields: []Field{{Key: "access_token", Label: "Graph access token", Secret: true, Required: true}, {Key: "from_user", Label: "From user ID", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := "https://graph.microsoft.com/v1.0/users/" + val(t, "from_user") + "/sendMail"
		msg := map[string]any{"message": map[string]any{"subject": m.Title, "body": map[string]any{"contentType": "Text", "content": m.Body}, "toRecipients": graphRecipients(val(t, "to"))}, "saveToSentItems": false}
		return postJSON(ctx, u, map[string]string{"Authorization": "Bearer " + val(t, "access_token")}, msg)
	}}
}

func smppBridgeProvider() Provider {
	return funcProvider{id: "smpp", name: "SMPP bridge", fields: []Field{{Key: "bridge_url", Label: "Bridge URL", Required: true}, {Key: "from", Label: "From"}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "bridge_url"), nil, map[string]any{"from": val(t, "from"), "to": val(t, "to"), "text": m.Title + "\n" + m.Body})
	}}
}

func smtp2goProvider() Provider {
	return funcProvider{id: "smtp2go", name: "SMTP2Go", fields: emailAPIFields("", "API key"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.smtp2go.com/v3/email/send", nil, map[string]any{"api_key": val(t, "api_key"), "sender": val(t, "from"), "to": splitCSV(val(t, "to")), "subject": m.Title, "text_body": m.Body})
	}}
}

func sendpulseProvider() Provider {
	return funcProvider{id: "sendpulse", name: "SendPulse SMTP API", fields: emailAPIFields("", "Bearer token"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.sendpulse.com/smtp/emails", map[string]string{"Authorization": "Bearer " + val(t, "api_key")}, map[string]any{"email": map[string]any{"subject": m.Title, "from": map[string]any{"email": val(t, "from")}, "to": emailList(val(t, "to")), "text": m.Body}})
	}}
}

func threemaProvider() Provider {
	return funcProvider{id: "threema", name: "Threema Gateway", fields: []Field{{Key: "gateway_id", Label: "Gateway ID", Required: true}, {Key: "secret", Label: "Secret", Secret: true, Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://msgapi.threema.ch/send_simple", nil, url.Values{"from": {val(t, "gateway_id")}, "secret": {val(t, "secret")}, "to": {val(t, "to")}, "text": {m.Title + "\n" + m.Body}})
	}}
}

func twitterProvider() Provider {
	return funcProvider{id: "twitter", name: "Twitter/X API v2", fields: []Field{{Key: "bearer_token", Label: "Bearer token", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		text := m.Title + "\n\n" + m.Body
		if len(text) > 280 {
			text = text[:277] + "..."
		}
		return postJSON(ctx, "https://api.twitter.com/2/tweets", map[string]string{"Authorization": "Bearer " + val(t, "bearer_token")}, map[string]any{"text": text})
	}}
}

func wechatProvider() Provider {
	return funcProvider{id: "wechat", name: "WeChat Work", fields: []Field{{Key: "corp_id", Label: "Corp ID", Required: true}, {Key: "secret", Label: "Secret", Secret: true, Required: true}, {Key: "agent_id", Label: "Agent ID", Required: true}, {Key: "to_user", Label: "To user"}}, send: func(ctx context.Context, t store.Target, m Message) error {
		var tok struct {
			AccessToken string `json:"access_token"`
			ErrCode     int    `json:"errcode"`
		}
		u := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=" + url.QueryEscape(val(t, "corp_id")) + "&corpsecret=" + url.QueryEscape(val(t, "secret"))
		if err := getJSON(ctx, u, &tok); err != nil {
			return err
		}
		to := val(t, "to_user")
		if to == "" {
			to = "@all"
		}
		agent, _ := strconv.Atoi(val(t, "agent_id"))
		return postJSON(ctx, "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token="+url.QueryEscape(tok.AccessToken), nil, map[string]any{"touser": to, "msgtype": "text", "agentid": agent, "text": map[string]any{"content": m.Title + "\n\n" + m.Body}})
	}}
}

func mqttPublish(ctx context.Context, address, clientID, topic, payload string) error {
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer c.Close()
	connectPayload := append(encMQTTString("MQTT"), []byte{4, 2, 0, 30}...)
	connectPayload = append(connectPayload, encMQTTString(clientID)...)
	if _, err := c.Write(append([]byte{0x10}, append(encRemain(len(connectPayload)), connectPayload...)...)); err != nil {
		return err
	}
	buf := make([]byte, 4)
	_, _ = c.Read(buf)
	pubPayload := append(encMQTTString(topic), []byte(payload)...)
	if _, err := c.Write(append([]byte{0x30}, append(encRemain(len(pubPayload)), pubPayload...)...)); err != nil {
		return err
	}
	_, err = c.Write([]byte{0xe0, 0x00})
	return err
}

func encMQTTString(s string) []byte {
	b := []byte(s)
	return append([]byte{byte(len(b) >> 8), byte(len(b))}, b...)
}

func encRemain(n int) []byte {
	var out []byte
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 128
		}
		out = append(out, digit)
		if n == 0 {
			return out
		}
	}
}

func graphRecipients(raw string) []map[string]any {
	var out []map[string]any
	for _, addr := range splitCSV(raw) {
		out = append(out, map[string]any{"emailAddress": map[string]any{"address": addr}})
	}
	return out
}
