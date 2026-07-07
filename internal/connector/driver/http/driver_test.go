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

	tc := &HTTPTransportConfig{
		URL:    ts.URL,
		Method: http.MethodPost,
		Headers: map[string]string{
			"Authorization": "Bearer test-token",
			"Content-Type":  "application/json",
		},
		Body: `{"to":"+1234567890","text":"Hello World"}`,
	}

	driver := NewDriver()
	resp, err := driver.Send(context.Background(), &connector.TransportRequest{
		Message: &domain.Message{Source: "SENDER", Destination: "+1234567890", Text: "Hello World"},
		Prepared: &domain.PreparedMessage{
			Destination: "+1234567890",
			Parts:       1,
			Encoding:    "gsm7",
		},
		Config: tc,
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
}

func TestDriver_Send_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid"}`))
	}))
	defer ts.Close()

	driver := NewDriver()
	resp, err := driver.Send(context.Background(), &connector.TransportRequest{
		Message:  &domain.Message{Destination: "+1234"},
		Prepared: &domain.PreparedMessage{Destination: "+1234", Parts: 1, Encoding: "gsm7"},
		Config:   &HTTPTransportConfig{URL: ts.URL},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.Status)
	}
}

func TestDriver_DecodeConfig(t *testing.T) {
	driver := NewDriver()
	data := []byte(`{"url":"https://api.example.com/send","method":"PUT","headers":{"Authorization":"Bearer x"}}`)
	cfg, err := driver.DecodeConfig(data)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}
	htc := cfg.(*HTTPTransportConfig)
	if htc.URL != "https://api.example.com/send" {
		t.Errorf("URL = %q", htc.URL)
	}
	if htc.Method != "PUT" {
		t.Errorf("Method = %q", htc.Method)
	}
	if htc.Headers["Authorization"] != "Bearer x" {
		t.Errorf("Authorization = %q", htc.Headers["Authorization"])
	}
}

func TestDriver_DecodeConfig_Empty(t *testing.T) {
	driver := NewDriver()
	cfg, err := driver.DecodeConfig(nil)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}
	htc := cfg.(*HTTPTransportConfig)
	if htc.Method != "POST" {
		t.Errorf("default method should be POST, got %q", htc.Method)
	}
}

func TestDecodeTransportConfig(t *testing.T) {
	driver := NewDriver()
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
			cfg, err := driver.DecodeConfig(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if cfg.(*HTTPTransportConfig).URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", cfg.(*HTTPTransportConfig).URL, tt.wantURL)
			}
		})
	}
}
