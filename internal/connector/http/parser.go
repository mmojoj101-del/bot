package httpconnector

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// ParseResponse converts an HTTP response into rule.ResponseData,
// evaluates the configured rules, and returns a domain.SendResult.
//
// Steps:
//  1. Read status code, headers, body
//  2. Attempt JSON parse of body
//  3. Build rule.ResponseData
//  4. Evaluate rules (accept/reject/retry/extract)
//  5. Convert rule.Result → domain.SendResult
func ParseResponse(
	resp *http.Response,
	body []byte,
	responseCfg ResponseConfig,
	ruleEngine *rule.Engine,
) *domain.SendResult {
	// 1. Build response data for rule engine
	respData := buildResponseData(resp, body)

	// 2. Evaluate rules
	rulesResult := ruleEngine.Evaluate(responseCfg.Rules, respData)

	// 3. Determine acceptance kind from rules
	acceptance := determineAcceptance(rulesResult)

	// 4. Extract fields from rule result
	externalID := rulesResult.Extract["external_id"]
	parts := 1        // default
	price := int64(0) // unknown until DLR

	if p := rulesResult.Extract["parts"]; p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			parts = v
		}
	}
	if p := rulesResult.Extract["price"]; p != "" {
		if v, err := strconv.ParseInt(p, 10, 64); err == nil {
			price = v
		}
	}

	// 5. Build SendResult
	return &domain.SendResult{
		ExternalID:     externalID,
		Parts:          parts,
		Price:          price,
		Cost:           price, // cost = price for now (handled by billing later)
		RawResponse:    body,
		ProviderStatus: rulesResult.Extract["provider_status"],
		Acceptance:     acceptance,
	}
}

// buildResponseData creates rule.ResponseData from HTTP response.
func buildResponseData(resp *http.Response, body []byte) rule.ResponseData {
	// Parse headers
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[strings.ToLower(k)] = resp.Header.Get(k)
	}

	// Attempt JSON parse if content type indicates JSON
	var parsed map[string]interface{}
	if len(body) > 0 {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") ||
			strings.Contains(contentType, "text/json") {
			_ = json.Unmarshal(body, &parsed)
		}
	}

	return rule.ResponseData{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    body,
		Parsed:  parsed,
	}
}

// determineAcceptance maps rule result to AcceptanceKind.
// Rule determines: accepted → Final, rejected → Rejected, retryable/unknown → PendingDLR
func determineAcceptance(r rule.Result) domain.AcceptanceKind {
	switch {
	case r.Accepted:
		return domain.AcceptanceFinal
	case r.Rejected:
		return domain.AcceptanceRejected
	case r.Retryable:
		return domain.AcceptancePendingDLR
	default:
		// No rule matched — conservative: mark as pending DLR (the pipeline
		// will wait for a DLR or timeout before marking as delivered/failed).
		return domain.AcceptancePendingDLR
	}
}
