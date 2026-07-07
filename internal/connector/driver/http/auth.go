package httpdriver

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/raghna/fury-sms-gateway/internal/connector"
)

// applyAuth decorates an HTTP request with authentication from the connector config.
func applyAuth(req *http.Request, transportReq *connector.TransportRequest) error {
	cfg := transportReq.Config
	if len(cfg) == 0 {
		return nil // no auth config
	}

	// Parse auth from the original ConnectorConfig (passed through rendered fields)
	authType := transportReq.RenderedFields["auth_type"]
	if authType == "" || authType == "none" {
		return nil
	}

	switch authType {
	case "bearer":
		token := transportReq.RenderedFields["auth_token"]
		if token == "" {
			return fmt.Errorf("bearer auth requires 'auth_token' template field")
		}
		req.Header.Set("Authorization", "Bearer "+token)

	case "basic":
		username := transportReq.RenderedFields["auth_username"]
		password := transportReq.RenderedFields["auth_password"]
		if username == "" || password == "" {
			return fmt.Errorf("basic auth requires 'auth_username' and 'auth_password' template fields")
		}
		req.SetBasicAuth(username, password)

	case "api_key":
		key := transportReq.RenderedFields["auth_key"]
		if key == "" {
			return fmt.Errorf("api_key auth requires 'auth_key' template field")
		}
		headerName := transportReq.RenderedFields["auth_header_name"]
		if headerName == "" {
			headerName = "X-API-Key"
		}
		prefix := transportReq.RenderedFields["auth_value_prefix"]

		if transportReq.RenderedFields["auth_encoding"] == "base64" {
			key = base64.StdEncoding.EncodeToString([]byte(key))
		}

		req.Header.Set(headerName, prefix+key)

	case "custom_header":
		headerName := transportReq.RenderedFields["auth_header_name"]
		value := transportReq.RenderedFields["auth_value"]
		if headerName == "" || value == "" {
			return fmt.Errorf("custom_header auth requires 'auth_header_name' and 'auth_value' template fields")
		}
		req.Header.Set(headerName, value)

	case "query_param":
		paramName := transportReq.RenderedFields["auth_param_name"]
		paramValue := transportReq.RenderedFields["auth_param_value"]
		if paramName == "" || paramValue == "" {
			return fmt.Errorf("query_param auth requires 'auth_param_name' and 'auth_param_value' template fields")
		}
		q := req.URL.Query()
		q.Set(paramName, paramValue)
		req.URL.RawQuery = q.Encode()

	default:
		return fmt.Errorf("unsupported auth type: %q", authType)
	}

	return nil
}
