package notify

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/RXWatcher/continuum-plugin-notifications/internal/store"
)

func smsGatewayProviders() []Provider {
	return []Provider{
		africasTalkingProvider(),
		bulkSMSProvider(),
		burstSMSProvider(),
		d7Provider(),
		kavenegarProvider(),
		msg91Provider(),
		sinchProvider(),
		sevenProvider(),
		smsManagerProvider(),
		smsEagleProvider(),
		freeMobileProvider(),
		fortySixElksProvider(),
		exotelProvider(),
		clickatellProvider(),
		octopushProvider(),
		sfrProvider(),
		voipmsProvider(),
		httpsmsProvider(),
	}
}

func africasTalkingProvider() Provider {
	return funcProvider{id: "africas_talking", name: "Africa's Talking SMS", fields: smsFields("Username", "API key"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://api.africastalking.com/version1/messaging", map[string]string{"apiKey": val(t, "token")}, url.Values{"username": {val(t, "account")}, "from": {val(t, "from")}, "to": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func bulkSMSProvider() Provider {
	return funcProvider{id: "bulksms", name: "BulkSMS", fields: smsFields("Username", "Password"), send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "account")+":"+val(t, "token")))
		return postJSON(ctx, "https://api.bulksms.com/v1/messages", map[string]string{"Authorization": auth}, map[string]any{"from": val(t, "from"), "to": val(t, "to"), "body": m.Title + "\n" + m.Body})
	}}
}

func burstSMSProvider() Provider {
	return funcProvider{id: "burstsms", name: "Burst SMS", fields: smsFields("API key", "API secret"), send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "account")+":"+val(t, "token")))
		return postForm(ctx, "https://api.transmitsms.com/send-sms.json", map[string]string{"Authorization": auth}, url.Values{"from": {val(t, "from")}, "to": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func d7Provider() Provider {
	return funcProvider{id: "d7networks", name: "D7 Networks", fields: []Field{{Key: "token", Label: "Bearer token", Secret: true, Required: true}, {Key: "from", Label: "From", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.d7networks.com/messages/v1/send", map[string]string{"Authorization": "Bearer " + val(t, "token")}, map[string]any{"messages": []any{map[string]any{"channel": "sms", "recipients": splitCSV(val(t, "to")), "content": m.Title + "\n" + m.Body, "msg_type": "text", "data_coding": "text"}}, "message_globals": map[string]any{"originator": val(t, "from")}})
	}}
}

func kavenegarProvider() Provider {
	return funcProvider{id: "kavenegar", name: "Kavenegar", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "from", Label: "From"}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		u := "https://api.kavenegar.com/v1/" + val(t, "api_key") + "/sms/send.json"
		return postForm(ctx, u, nil, url.Values{"sender": {val(t, "from")}, "receptor": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func msg91Provider() Provider {
	return funcProvider{id: "msg91", name: "MSG91", fields: []Field{{Key: "auth_key", Label: "Auth key", Secret: true, Required: true}, {Key: "template_id", Label: "Template ID"}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://control.msg91.com/api/v5/flow", map[string]string{"authkey": val(t, "auth_key")}, map[string]any{"template_id": val(t, "template_id"), "short_url": "0", "recipients": []any{map[string]any{"mobiles": val(t, "to"), "message": m.Title + "\n" + m.Body}}})
	}}
}

func sinchProvider() Provider {
	return funcProvider{id: "sinch", name: "Sinch SMS", fields: smsFields("Service plan ID", "API token"), send: func(ctx context.Context, t store.Target, m Message) error {
		u := "https://us.sms.api.sinch.com/xms/v1/" + val(t, "account") + "/batches"
		return postJSON(ctx, u, map[string]string{"Authorization": "Bearer " + val(t, "token")}, map[string]any{"from": val(t, "from"), "to": splitCSV(val(t, "to")), "body": m.Title + "\n" + m.Body})
	}}
}

func sevenProvider() Provider {
	return funcProvider{id: "seven", name: "Seven SMS", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "from", Label: "From"}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://gateway.seven.io/api/sms", map[string]string{"X-Api-Key": val(t, "api_key")}, url.Values{"from": {val(t, "from")}, "to": {val(t, "to")}, "text": {m.Title + "\n" + m.Body}})
	}}
}

func smsManagerProvider() Provider {
	return funcProvider{id: "smsmanager", name: "SMS Manager", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://http-api.smsmanager.cz/Send", nil, url.Values{"apikey": {val(t, "api_key")}, "number": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func smsEagleProvider() Provider {
	return funcProvider{id: "smseagle", name: "SMSEagle", fields: []Field{{Key: "server", Label: "Server", Required: true}, {Key: "token", Label: "Token", Secret: true, Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, strings.TrimRight(val(t, "server"), "/")+"/api/v2/messages/sms", map[string]string{"access-token": val(t, "token")}, map[string]any{"to": splitCSV(val(t, "to")), "text": m.Title + "\n" + m.Body})
	}}
}

func freeMobileProvider() Provider {
	return funcProvider{id: "freemobile", name: "Free-Mobile", fields: []Field{{Key: "user", Label: "User", Required: true}, {Key: "password", Label: "Password", Secret: true, Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://smsapi.free-mobile.fr/sendmsg", nil, url.Values{"user": {val(t, "user")}, "pass": {val(t, "password")}, "msg": {m.Title + "\n" + m.Body}})
	}}
}

func fortySixElksProvider() Provider {
	return funcProvider{id: "fortysixelks", name: "46elks", fields: smsFields("Username", "Password"), send: func(ctx context.Context, t store.Target, m Message) error {
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "account")+":"+val(t, "token")))
		return postForm(ctx, "https://api.46elks.com/a1/sms", map[string]string{"Authorization": auth}, url.Values{"from": {val(t, "from")}, "to": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func exotelProvider() Provider {
	return funcProvider{id: "exotel", name: "Exotel", fields: smsFields("Account SID", "Auth token"), send: func(ctx context.Context, t store.Target, m Message) error {
		u := "https://api.exotel.com/v1/Accounts/" + val(t, "account") + "/Sms/send.json"
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(val(t, "account")+":"+val(t, "token")))
		return postForm(ctx, u, map[string]string{"Authorization": auth}, url.Values{"From": {val(t, "from")}, "To": {val(t, "to")}, "Body": {m.Title + "\n" + m.Body}})
	}}
}

func clickatellProvider() Provider {
	return funcProvider{id: "clickatell", name: "Clickatell", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://platform.clickatell.com/messages", map[string]string{"Authorization": val(t, "api_key")}, map[string]any{"content": m.Title + "\n" + m.Body, "to": splitCSV(val(t, "to"))})
	}}
}

func octopushProvider() Provider {
	return funcProvider{id: "octopush", name: "Octopush", fields: smsFields("Login", "API key"), send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.octopush.com/v1/public/sms-campaign/send", nil, map[string]any{"user_login": val(t, "account"), "api_key": val(t, "token"), "sms_text": m.Title + "\n" + m.Body, "sms_recipients": splitCSV(val(t, "to")), "sms_sender": val(t, "from")})
	}}
}

func sfrProvider() Provider {
	return funcProvider{id: "sfr", name: "SFR", fields: []Field{{Key: "webhook_url", Label: "Webhook URL", Required: true}, {Key: "authorization", Label: "Authorization", Secret: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, val(t, "webhook_url"), map[string]string{"Authorization": val(t, "authorization")}, map[string]any{"message": m.Title + "\n" + m.Body})
	}}
}

func voipmsProvider() Provider {
	return funcProvider{id: "voipms", name: "VoIP.ms SMS", fields: []Field{{Key: "username", Label: "Username", Required: true}, {Key: "password", Label: "Password", Secret: true, Required: true}, {Key: "did", Label: "DID", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postForm(ctx, "https://voip.ms/api/v1/rest.php", nil, url.Values{"api_username": {val(t, "username")}, "api_password": {val(t, "password")}, "method": {"sendSMS"}, "did": {val(t, "did")}, "dst": {val(t, "to")}, "message": {m.Title + "\n" + m.Body}})
	}}
}

func httpsmsProvider() Provider {
	return funcProvider{id: "httpsms", name: "httpSMS", fields: []Field{{Key: "api_key", Label: "API key", Secret: true, Required: true}, {Key: "from", Label: "From", Required: true}, {Key: "to", Label: "To", Required: true}}, send: func(ctx context.Context, t store.Target, m Message) error {
		return postJSON(ctx, "https://api.httpsms.com/v1/messages/send", map[string]string{"x-api-key": val(t, "api_key")}, map[string]any{"from": val(t, "from"), "to": val(t, "to"), "content": m.Title + "\n" + m.Body})
	}}
}
