// Package rule provides a generic condition-action rule engine for
// evaluating protocol responses (HTTP status, SMPP PDU fields, etc.).
//
// All protocol connectors use the same rule engine to determine:
//   - Accept: message sent successfully
//   - Reject: provider rejected the message
//   - Retry: temporary failure, should retry
//   - Extract: capture fields from response (external_id, price, etc.)
//
// Rules are defined in EndpointConfig (JSONB) and evaluated at runtime.
// No code changes needed when providers change their response format.
package rule

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Condition checks a response field against an expected value.
type Condition struct {
	// Field is the response field to check.
	//   - "status"          → response status code (HTTP status, SMPP command_status)
	//   - "header.X-Name"   → response header (HTTP only)
	//   - "body.path.field" → JSON field in response body
	//   - "body"            → raw body string
	//   - "fields.key"      → Fields map set by GenericConnector
	Field string `json:"field"`

	// Operator is the comparison operator.
	//   "eq"       → Field == Value (string or numeric)
	//   "neq"      → Field != Value
	//   "gt"       → Field > Value  (numeric)
	//   "gte"      → Field >= Value (numeric)
	//   "lt"       → Field < Value  (numeric)
	//   "lte"      → Field <= Value (numeric)
	//   "contains" → strings.Contains(Field, Value)
	//   "exists"   → Field is present and non-empty (Value is ignored)
	//   "in"       → Field is one of the comma-separated Values
	Operator string `json:"operator"`

	// Value is the expected value (or comma-separated list for "in").
	Value string `json:"value"`
}

// Action is executed when a Condition evaluates to true.
type Action struct {
	// Type of action:
	//   "accept"        → message is accepted (SuccessResult)
	//   "reject"        → message is rejected (failure)
	//   "retry"         → retry the message (temporary failure)
	//   "extract"       → extract Value from response into ExtractResult[Key]
	//   "set"           → set Key to Value in ExtractResult
	Type string `json:"type"`

	// Key is the target key for extract/set actions.
	// For extract: the key in the Extract result map.
	// For set: the key in the Extract result map.
	Key string `json:"key,omitempty"`

	// Value is the field path for extract actions, or literal for set actions.
	// For extract with empty Value, tries Key as field path.
	Value string `json:"value,omitempty"`
}

// Rule pairs a Condition with one or more Actions.
// The first matching rule sets the terminal decision (accept/reject/retry).
// Extract/set actions from subsequent rules continue to run.
type Rule struct {
	// Name is optional — for debugging and UI display.
	Name      string   `json:"name"`
	Condition Condition `json:"condition"`
	Actions   []Action  `json:"actions"`
}

// ResponseData is the parsed response data available to rules.
// Generic across all protocols — HTTP, SMPP, SIP, Kafka, AMQP, etc.
type ResponseData struct {
	// Status is the protocol status code (HTTP status, SMPP command_status).
	Status int

	// Headers is the response headers (HTTP) or metadata map.
	Headers map[string]string

	// Body is the raw response body.
	Body []byte

	// Parsed is the JSON-decoded body, if applicable.
	Parsed map[string]interface{}

	// Fields holds additional protocol-specific context.
	// Set by GenericConnector before calling Evaluate.
	// Examples:
	//   "body.message_id" → "ext-123"
	//   "header.content_type" → "application/json"
	//   "smpp.command_status" → "ESME_ROK"
	//   "latency_ms" → "42"
	Fields map[string]string
}

// Result holds the outcome of rule evaluation.
type Result struct {
	// Accepted is true if an "accept" action matched.
	Accepted bool

	// Rejected is true if a "reject" action matched.
	Rejected bool

	// Retryable is true if a "retry" action matched.
	Retryable bool

	// Extract holds key-value pairs collected by "extract" and "set" actions.
	Extract map[string]string
}

// Engine evaluates rules against response data.
// Stateless and thread-safe — instantiate once and reuse.
type Engine struct{}

// NewEngine creates a rule engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Evaluate runs all rules in order and returns the combined result.
// The first matching rule determines the terminal decision (accept/reject/retry).
// Extract/set actions from all matching rules continue — they don't stop.
func (e *Engine) Evaluate(rules []Rule, resp ResponseData) Result {
	result := Result{
		Extract: make(map[string]string),
	}

	decided := false

	for _, rule := range rules {
		matched, err := e.evaluateCondition(rule.Condition, resp)
		if err != nil || !matched {
			continue
		}

		for _, action := range rule.Actions {
			switch action.Type {
			case "accept":
				if !decided {
					result.Accepted = true
					decided = true
				}
			case "reject":
				if !decided {
					result.Rejected = true
					decided = true
				}
			case "retry":
				if !decided {
					result.Retryable = true
					decided = true
				}
			case "extract":
				path := action.Value
				if path == "" {
					path = action.Key
				}
				if val := e.extractField(resp, path); val != "" {
					result.Extract[action.Key] = val
				} else if !strings.Contains(path, ".") {
					// Bare field name without prefix: try body.<field>
					if val := e.extractField(resp, "body."+path); val != "" {
						result.Extract[action.Key] = val
					}
				}
			case "set":
				result.Extract[action.Key] = action.Value
			}
		}

		// Terminal decision stops decision rules but not extract/set.
		if decided {
			break
		}
	}

	return result
}

// evaluateCondition checks a single condition against response data.
func (e *Engine) evaluateCondition(c Condition, resp ResponseData) (bool, error) {
	fieldVal := e.extractField(resp, c.Field)

	switch c.Operator {
	case "exists":
		return fieldVal != "", nil

	case "eq":
		return e.normalize(fieldVal) == e.normalize(c.Value), nil

	case "neq":
		return e.normalize(fieldVal) != e.normalize(c.Value), nil

	case "contains":
		return strings.Contains(strings.ToLower(fieldVal), strings.ToLower(c.Value)), nil

	case "gt":
		fv, err := strconv.ParseFloat(fieldVal, 64)
		if err != nil {
			return false, fmt.Errorf("gt: field %q value %q is not numeric", c.Field, fieldVal)
		}
		cv, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return false, fmt.Errorf("gt: condition value %q is not numeric", c.Value)
		}
		return fv > cv, nil

	case "gte":
		fv, err := strconv.ParseFloat(fieldVal, 64)
		if err != nil {
			return false, fmt.Errorf("gte: field %q value %q is not numeric", c.Field, fieldVal)
		}
		cv, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return false, fmt.Errorf("gte: condition value %q is not numeric", c.Value)
		}
		return fv >= cv, nil

	case "lt":
		fv, err := strconv.ParseFloat(fieldVal, 64)
		if err != nil {
			return false, fmt.Errorf("lt: field %q value %q is not numeric", c.Field, fieldVal)
		}
		cv, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return false, fmt.Errorf("lt: condition value %q is not numeric", c.Value)
		}
		return fv < cv, nil

	case "lte":
		fv, err := strconv.ParseFloat(fieldVal, 64)
		if err != nil {
			return false, fmt.Errorf("lte: field %q value %q is not numeric", c.Field, fieldVal)
		}
		cv, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return false, fmt.Errorf("lte: condition value %q is not numeric", c.Value)
		}
		return fv <= cv, nil

	case "in":
		vals := strings.Split(c.Value, ",")
		for _, v := range vals {
			if e.normalize(fieldVal) == e.normalize(strings.TrimSpace(v)) {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, fmt.Errorf("unknown operator: %q", c.Operator)
	}
}

// extractField retrieves a field value from response data using dot-path notation.
// Supported paths:
//   - "status"              → resp.Status
//   - "header.X-Name"       → resp.Headers["X-Name"]
//   - "body.path.to.field"  → JSON navigation in resp.Parsed
//   - "body"                → string(resp.Body)
//   - "fields.key"          → resp.Fields["key"]
//   - "key" (no prefix)     → tries resp.Fields["key"], then body.key
func (e *Engine) extractField(resp ResponseData, path string) string {
	if path == "" {
		return ""
	}

	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 0 {
		return ""
	}

	switch parts[0] {
	case "status":
		return strconv.Itoa(resp.Status)

	case "header":
		if len(parts) < 2 {
			return ""
		}
		return resp.Headers[parts[1]]

	case "body":
		if len(parts) < 2 {
			return string(resp.Body)
		}
		return e.navigateJSON(resp, parts[1])

	case "fields":
		if len(parts) < 2 {
			return ""
		}
		if resp.Fields != nil {
			return resp.Fields[parts[1]]
		}
		return ""
	}

	// No prefix: try Fields, then body.<path> as fallback
	if resp.Fields != nil {
		if v, ok := resp.Fields[path]; ok {
			return v
		}
	}
	return e.navigateJSON(resp, path)
}

// navigateJSON follows a dot-path into a parsed JSON structure.
func (e *Engine) navigateJSON(resp ResponseData, path string) string {
	if resp.Parsed == nil {
		if len(resp.Body) > 0 {
			var parsed map[string]interface{}
			if err := json.Unmarshal(resp.Body, &parsed); err == nil {
				resp.Parsed = parsed
			}
		}
		if resp.Parsed == nil {
			return ""
		}
	}

	parts := strings.Split(path, ".")
	current := interface{}(resp.Parsed)

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}

	switch v := current.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// normalize trims spaces and lowercases for comparison.
func (e *Engine) normalize(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
