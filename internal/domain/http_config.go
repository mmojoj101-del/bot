package domain

// HTTPConnectorConfig represents the JSONB config for an HTTP connector.
type HTTPConnectorConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"` // POST, GET, PUT
	ContentType  string            `json:"content_type"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyTemplate string            `json:"body_template"` // Go text/template with {{.Source}}, {{.Destination}}, {{.Text}}
	AuthType     string            `json:"auth_type"`     // none, bearer, basic, api_key
	AuthToken    string            `json:"auth_token,omitempty"`
	AuthUsername string            `json:"auth_username,omitempty"`
	AuthPassword string            `json:"auth_password,omitempty"`
	TimeoutSec   int               `json:"timeout_sec"`          // default 30
	RetryOnFail  bool              `json:"retry_on_fail"`        // auto-retry on 5xx
	SuccessCodes []int             `json:"success_codes"`        // e.g., [200, 202]
	// JSONPath or template to extract external_id from response
	ExternalIDPath string `json:"external_id_path"`
	// JSONPath or template to extract provider status from response
	StatusPath string `json:"status_path"`
}
