package connector

import (
	"testing"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func TestMemoryRegistry_New(t *testing.T) {
	r := NewMemoryRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.Len() != 0 {
		t.Errorf("Len() = %d, want 0", r.Len())
	}
}

func TestMemoryRegistry_AddAndGet(t *testing.T) {
	r := NewMemoryRegistry()
	c := NewMockConnector("http-1", domain.ConnectorTypeHTTPClient)

	err := r.Add(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := r.Get("http-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID() != "http-1" {
		t.Errorf("ID() = %q, want http-1", got.ID())
	}
	if got.Protocol() != domain.ConnectorTypeHTTPClient {
		t.Errorf("Protocol() = %q, want http_client", got.Protocol())
	}
}

func TestMemoryRegistry_Get_NotFound(t *testing.T) {
	r := NewMemoryRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent connector")
	}
}

func TestMemoryRegistry_Add_Duplicate(t *testing.T) {
	r := NewMemoryRegistry()
	c1 := NewMockConnector("dup", domain.ConnectorTypeHTTPClient)
	c2 := NewMockConnector("dup", domain.ConnectorTypeSMPPClient)

	if err := r.Add(c1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Add(c2); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestMemoryRegistry_Remove(t *testing.T) {
	r := NewMemoryRegistry()
	r.MustAdd(NewMockConnector("http-1", domain.ConnectorTypeHTTPClient))

	r.Remove("http-1")
	_, err := r.Get("http-1")
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestMemoryRegistry_List(t *testing.T) {
	r := NewMemoryRegistry()
	r.MustAdd(NewMockConnector("http-1", domain.ConnectorTypeHTTPClient))
	r.MustAdd(NewMockConnector("smpp-1", domain.ConnectorTypeSMPPClient))

	items := r.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestMemoryRegistry_Len(t *testing.T) {
	r := NewMemoryRegistry()
	r.MustAdd(NewMockConnector("a", domain.ConnectorTypeHTTPClient))
	r.MustAdd(NewMockConnector("b", domain.ConnectorTypeSMPPClient))

	if r.Len() != 2 {
		t.Errorf("Len() = %d, want 2", r.Len())
	}
}
