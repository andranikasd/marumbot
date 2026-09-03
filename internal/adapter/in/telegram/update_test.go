package telegram

import (
	"encoding/json"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := map[string]struct{ kind, arg string }{
		"/start":                  {KindStart, ""},
		"/help":                   {KindHelp, ""},
		"/loans":                  {KindLoans, ""},
		"/add":                    {KindAdd, ""},
		"/budget 150000":          {KindBudget, "150000"},
		"/language":               {KindLanguage, ""},
		"/lang en":                {KindLanguage, "en"},
		"  /START  ":              {KindStart, ""},
		"/help@marum_dev_bot":     {KindHelp, ""},
		"/budget@marum_dev_bot 1": {KindBudget, "1"},
		"/nonsense":               {KindIgnore, ""},
		"hello":                   {KindText, "hello"},
		"":                        {KindIgnore, ""},
		"   ":                     {KindIgnore, ""},
	}
	for in, want := range cases {
		kind, arg := classify(in)
		if kind != want.kind || arg != want.arg {
			t.Errorf("classify(%q) = (%s, %q), want (%s, %q)", in, kind, arg, want.kind, want.arg)
		}
	}
}

func TestNormaliseMessage(t *testing.T) {
	var u Update
	raw := `{"update_id":11,"message":{"message_id":5,"date":1,
	         "from":{"id":42,"is_bot":false,"language_code":"hy-AM"},
	         "chat":{"id":42,"type":"private"},"text":"/add"}}`
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	n, ok := Normalise(u)
	if !ok {
		t.Fatal("expected the update to normalise")
	}
	if n.Kind != KindAdd || n.UserID != 42 || n.ChatID != 42 || n.Language != "hy-AM" {
		t.Errorf("got %+v", n)
	}
}

func TestNormaliseCallback(t *testing.T) {
	var u Update
	raw := `{"update_id":12,"callback_query":{"id":"c1","data":"lang:en",
	         "from":{"id":7,"is_bot":false},"message":{"chat":{"id":7,"type":"private"}}}}`
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	n, ok := Normalise(u)
	if !ok {
		t.Fatal("expected the update to normalise")
	}
	if n.Kind != KindCallback || n.UserID != 7 || n.ChatID != 7 {
		t.Errorf("got %+v", n)
	}
	var p payload
	if err := json.Unmarshal(n.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Data != "lang:en" {
		t.Errorf("payload data = %q", p.Data)
	}
}

// The stored payload must not carry anything that identifies the sender. Those
// identifiers live encrypted in identities, and copying them into a jsonb column
// alongside financial data would defeat the separation entirely.
func TestPayloadCarriesNoIdentifiers(t *testing.T) {
	var u Update
	raw := `{"update_id":13,"message":{"message_id":9,"date":1,
	         "from":{"id":8982118495,"is_bot":false,"language_code":"en"},
	         "chat":{"id":8982118495,"type":"private"},"text":"hello"}}`
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	n, ok := Normalise(u)
	if !ok {
		t.Fatal("expected the private message to normalise")
	}
	if s := string(n.Payload); contains(s, "8982118495") {
		t.Errorf("payload %s contains the telegram id", s)
	}
}

func contains(h, n string) bool {
	return len(h) >= len(n) && func() bool {
		for i := 0; i+len(n) <= len(h); i++ {
			if h[i:i+len(n)] == n {
				return true
			}
		}
		return false
	}()
}

func TestBotsAreIgnored(t *testing.T) {
	var u Update
	raw := `{"update_id":14,"message":{"message_id":1,"date":1,
	         "from":{"id":1,"is_bot":true},"chat":{"id":1,"type":"private"},"text":"/start"}}`
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	if _, ok := Normalise(u); ok {
		t.Error("an update from a bot was accepted")
	}
}

func TestEmptyUpdateIsIgnored(t *testing.T) {
	if _, ok := Normalise(Update{UpdateID: 1}); ok {
		t.Error("an update with no message and no callback was accepted")
	}
}

func TestNormalisePrivateChatProtection(t *testing.T) {
	cases := []struct {
		name  string
		from  string
		chat  string
		extra string
		want  bool
	}{
		{name: "private", want: true},
		{name: "group", chat: `{"id":-42,"type":"group"}`},
		{name: "supergroup", chat: `{"id":-42,"type":"supergroup"}`},
		{name: "channel", chat: `{"id":-42,"type":"channel"}`},
		{name: "group matching sender", chat: `{"id":42,"type":"group"}`},
		{name: "missing type", chat: `{"id":42}`},
		{name: "unknown type", chat: `{"id":42,"type":"unknown"}`},
		{name: "different private owner", chat: `{"id":43,"type":"private"}`},
		{name: "missing chat", chat: `null`},
		{name: "missing sender", from: `null`},
		{name: "bot", from: `{"id":42,"is_bot":true}`},
		{name: "missing sender id", from: `{}`},
		{name: "zero ids", from: `{"id":0}`, chat: `{"id":0,"type":"private"}`},
		{name: "negative ids", from: `{"id":-42}`, chat: `{"id":-42,"type":"private"}`},
		{name: "forwarded group", extra: `,"forward_origin":{"type":"chat","sender_chat":{"id":-42,"type":"supergroup"}}`},
		{name: "forwarded channel", extra: `,"forward_origin":{"type":"channel","chat":{"id":-42,"type":"channel"}}`},
		{name: "legacy forwarded group", extra: `,"forward_from_chat":{"id":-42,"type":"group"}`},
		{name: "automatic forward", extra: `,"is_automatic_forward":true`},
		{name: "unknown forward origin", extra: `,"forward_origin":{"type":"unknown"}`},
		{name: "missing forward origin type", extra: `,"forward_origin":{}`},
		{name: "forwarded user", extra: `,"forward_origin":{"type":"user"}`, want: true},
		{name: "forwarded hidden user", extra: `,"forward_origin":{"type":"hidden_user"}`, want: true},
	}
	for _, tc := range cases {
		for _, callback := range []bool{false, true} {
			kind := KindHelp
			if callback {
				kind = KindCallback
			}
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				from, chat := tc.from, tc.chat
				if from == "" {
					from = `{"id":42,"is_bot":false,"language_code":"hy"}`
				}
				if chat == "" {
					chat = `{"id":42,"type":"private"}`
				}
				raw := `{"message":{"message_id":5,"from":` + from + `,"chat":` + chat + `,"text":"/help@marum_dev_bot"` + tc.extra + `}}`
				wantPayload := `{"text":"/help@marum_dev_bot","message_id":5}`
				if callback {
					raw = `{"callback_query":{"id":"c1","from":` + from + `,"data":"lang:en","message":{"from":{"id":99,"is_bot":true},"chat":` + chat + tc.extra + `}}}`
					wantPayload = `{"data":"lang:en"}`
				}
				var u Update
				if err := json.Unmarshal([]byte(raw), &u); err != nil {
					t.Fatal(err)
				}
				n, ok := Normalise(u)
				if ok != tc.want {
					t.Fatalf("accepted = %v, want %v", ok, tc.want)
				}
				if !ok {
					if n.Kind != "" || n.UserID != 0 || n.ChatID != 0 || n.Language != "" || n.Payload != nil {
						t.Fatalf("rejected update returned command: %+v", n)
					}
					return
				}
				if n.Kind != kind || n.UserID != 42 || n.ChatID != 42 || n.Language != "hy" || string(n.Payload) != wantPayload {
					t.Fatalf("unexpected private command: %+v, payload %s", n, n.Payload)
				}
			})
		}
	}
}

func TestNormaliseCallbackWithoutMessage(t *testing.T) {
	for _, raw := range []string{
		`{"callback_query":{"from":{"id":42},"data":"lang:en","inline_message_id":"inline-1"}}`,
		`{"callback_query":{"from":{"id":42},"data":"lang:en","message":null}}`,
	} {
		var u Update
		if err := json.Unmarshal([]byte(raw), &u); err != nil {
			t.Fatal(err)
		}
		if _, ok := Normalise(u); ok {
			t.Fatal("callback without a message was accepted")
		}
	}
}
