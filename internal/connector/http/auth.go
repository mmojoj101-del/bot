package httpconnector

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

// applyAuth decorates an HTTP request with authentication from the config.
func applyAuth(req *http.Request, cfg AuthConfig) error {
	switch cfg.Type {
	case "", "none":
		return nil

	case "bearer":
		token := cfg.Credentials["token"]
		if token == "" {
			return fmt.Errorf("bearer auth requires 'token' credential")
		}
		req.Header.Set("Authorization", "Bearer "+token)

	case "basic":
		username := cfg.Credentials["username"]
		password := cfg.Credentials["password"]
		if username == "" || password == "" {
			return fmt.Errorf("basic auth requires 'username' and 'password' credentials")
		}
		req.SetBasicAuth(username, password)

	case "api_key":
		key := cfg.Credentials["key"]
		if key == "" {
			return fmt.Errorf("api_key auth requires 'key' credential")
		}

		// Optional: custom header name (default: X-API-Key)
		headerName := cfg.Credentials["header_name"]
		if headerName == "" {
			headerName = "X-API-Key"
		}

		// Optional: value prefix like "Bearer " or "Basic "
		prefix := cfg.Credentials["value_prefix"]

		// Optional: encoding
		if cfg.Credentials["encoding"] == "base64" {
			key = base64.StdEncoding.EncodeToString([]byte(key))
		}

		req.Header.Set(headerName, prefix+key)

	case "custom_header":
		headerName := cfg.Credentials["header_name"]
		value := cfg.Credentials["value"]
		if headerName == "" || value == "" {
			return fmt.Errorf("custom_header auth requires 'header_name' and 'value' credentials")
		}
		req.Header.Set(headerName, value)

	case "query_param":
		paramName := cfg.Credentials["param_name"]
		paramValue := cfg.Credentials["param_value"]
		if paramName == "" || paramValue == "" {
			return fmt.Errorf("query_param auth requires 'param_name' and 'param_value' credentials")
		}
		q := req.URL.Query()
		q.Set(paramName, paramValue)
		req.URL.RawQuery = q.Encode()

	default:
		return fmt.Errorf("unsupported auth type: %q", cfg.Type)
	}

	return nil
}
