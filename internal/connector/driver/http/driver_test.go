package httpdriver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func TestDriver_Send_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %q", r.Header.Get("Content-Type"))
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["to"] != "+1234567890" {
			t.Errorf("expected to = +1234567890, got %v", body["to"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Message-Id", "prov-msg-123")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	tc := TransportConfig{
		URL:    ts.URL,
		Method: http.MethodPost,
		Headers: []KeyValue{
			{Key: "Authorization", Value: "Bearer test-token"},
		},
		Body: &BodyConfig{
			Template:    `{"to":"{{destination}}","text":"{{text}}"}`,
			ContentType: "application/json",
		},
	}
	tcBytes, _ := json.Marshal(tc)

	driver := NewDriver()
	resp, err := driver.Send(context.Background(), &connector.TransportRequest{
		Message: &domain.Message{
			Source:      "SENDER",
			Destination: "+1234567890",
			Text:       "Hello World",
		},
		Prepared: &domain.PreparedMessage{
			Destination: "+1234567890",
			Parts:       1,
			Encoding:    "gsm7",
		},
		Config: tcBytes,
		RenderedFields: map[string]string{
			"destination": "+1234567890",
			"text":       "Hello World",
			"source":     "SENDER",
			"auth_type":  "bearer",
			"auth_token": "test-token",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Status)
	}
	if resp.ExternalID != "prov-msg-123" {
		t.Errorf("expected ExternalID = prov-msg-123, got %q", resp.ExternalID)
	}
	if len(resp.Body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestDriver_Send_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid request"}`))
	}))
	defer ts.Close()

	tc := TransportConfig{URL: ts.URL}
	tcBytes, _ := json.Marshal(tc)

	driver := NewDriver()
	resp, err := driver.Send(context.Background(), &connector.TransportRequest{
		Message: &domain.Message{Destination: "+1234"},
		Prepared: &domain.PreparedMessage{
			Destination: "+1234",
			Parts:       1,
			Encoding:    "gsm7",
		},
		Config: tcBytes,
		RenderedFields: map[string]string{
			"destination": "+1234",
			"text":       "Hello",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
}

func TestDriver_Send_QueryParams(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "abc123" {
			t.Errorf("expected api_key=abc123, got %q", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("ref") != "msg-001" {
			t.Errorf("expected ref=msg-001, got %q", r.URL.Query().Get("ref"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tc := TransportConfig{
		URL:    ts.URL,
		Method: http.MethodGet,
		QueryParams: []KeyValue{
			{Key: "api_key", Value: "abc123"},
			{Key: "ref", Value: "{{message_id}}"},
		},
	}
	tcBytes, _ := json.Marshal(tc)

	driver := NewDriver()
	resp, err := driver.Send(context.Background(), &connector.TransportRequest{
		Message:  &domain.Message{},
		Prepared: &domain.PreparedMessage{Destination: "+1234", Parts: 1, Encoding: "gsm7"},
		Config:   tcBytes,
		RenderedFields: map[string]string{
			"message_id": "msg-001",
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.Status)
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		fields map[string]string
		want   string
	}{
		{"no templates", "hello", nil, "hello"},
		{"single field", "{{destination}}", map[string]string{"destination": "+1234"}, "+1234"},
		{"multiple fields", "{{source}}:{{text}}", map[string]string{"source": "APP", "text": "Hello"}, "APP:Hello"},
		{"missing field", "{{missing}}", map[string]string{"other": "x"}, "{{missing}}"},
		{"mixed", "to={{dest}}&text={{msg}}", map[string]string{"dest": "+1234", "msg": "hi"}, "to=+1234&text=hi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render(tt.s, tt.fields); got != tt.want {
				t.Errorf("render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeTransportConfig(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantURL string
		wantErr bool
	}{
		{"empty", nil, "", false},
		{"valid", []byte(`{"url":"https://api.example.com/send","method":"PUT"}`), "https://api.example.com/send", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, err := decodeTransportConfig(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tc.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", tc.URL, tt.wantURL)
			}
		})
	}
}
