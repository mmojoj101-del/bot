// Package template provides a shared template engine for rendering
// message variables into protocol-specific payloads.
//
// All protocol connectors (HTTP, SMPP, SIP) use this engine to render
// configurable templates stored in EndpointConfig.
//
// Variables:
//
//	{{Source}}      — sender address/number
//	{{Destination}} — recipient address/number (normalized)
//	{{Text}}        — message content
//	{{Parts}}       — number of SMS parts
//	{{Encoding}}    — message encoding (GSM7, UCS2)
//	{{ClientRef}}   — client reference
//	{{MessageID}}   — internal message ID
//	{{TenantID}}    — tenant identifier
//	{{Now}}         — current timestamp (RFC3339)
//	{{UUID}}        — random UUID v4
//	{{Custom KEY}}  — custom fields from provider config
//
// Usage:
//
//	eng := template.NewEngine()
//	rendered, err := eng.Render("{\"to\":\"{{Destination}}\",\"text\":\"{{Text}}\"}", data)
package template

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
)

// Data carries all variables available to templates.
type Data struct {
	Source      string
	Destination string
	Text        string
	Parts       int
	Encoding    string
	ClientRef   string
	MessageID   string
	TenantID    string

	// Custom key-value pairs from EndpointConfig or provider data.
	Custom map[string]string
}

// FuncMap provides template functions available in all templates.
var FuncMap = template.FuncMap{
	"now":      func() string { return time.Now().UTC().Format(time.RFC3339) },
	"uuid":     func() string { return uuid.New().String() },
	"upper":    strings.ToUpper,
	"lower":    strings.ToLower,
	"trim":     strings.TrimSpace,
	"urlencode": func(s string) string { return strings.ReplaceAll(s, " ", "%20") },
}

// Engine caches parsed templates and renders them with variable substitution.
// Thread-safe: uses sync.Map for the template cache.
type Engine struct {
	cache sync.Map // map[string]*template.Template
}

// NewEngine creates a template engine with an empty cache.
func NewEngine() *Engine {
	return &Engine{}
}

// Render renders a template string and returns the result.
func (e *Engine) Render(tmpl string, data Data) (string, error) {
	t, err := e.parseOrGet(tmpl)
	if err != nil {
		return "", err
	}

	ctx := e.buildContext(data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// RenderBytes is like Render but returns []byte for body construction.
func (e *Engine) RenderBytes(tmpl string, data Data) ([]byte, error) {
	s, err := e.Render(tmpl, data)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// parseOrGet returns a cached template or parses and caches it.
func (e *Engine) parseOrGet(tmpl string) (*template.Template, error) {
	if cached, ok := e.cache.Load(tmpl); ok {
		return cached.(*template.Template), nil
	}

	parsed, err := template.New("msg").Funcs(FuncMap).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	e.cache.Store(tmpl, parsed)
	return parsed, nil
}

// buildContext constructs the template data map from Data.
func (e *Engine) buildContext(data Data) map[string]interface{} {
	ctx := map[string]interface{}{
		"Source":      data.Source,
		"Destination": data.Destination,
		"Text":        data.Text,
		"Parts":       data.Parts,
		"Encoding":    data.Encoding,
		"ClientRef":   data.ClientRef,
		"MessageID":   data.MessageID,
		"TenantID":    data.TenantID,
	}

	// Add custom fields at top level for easy access: {{api_key}}, {{token}}
	for k, v := range data.Custom {
		ctx[k] = v
	}

	return ctx
}

// MustRender panics on error — for static templates that must compile.
func MustRender(eng *Engine, tmpl string, data Data) string {
	s, err := eng.Render(tmpl, data)
	if err != nil {
		panic(err)
	}
	return s
}
