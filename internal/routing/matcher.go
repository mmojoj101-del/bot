package routing

import (
	"strings"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// RouteMatcher filters and ranks routes for a given message.
// It implements the longest-prefix-match algorithm for message destinations.
type RouteMatcher struct{}

// Match returns routes that match the message, ordered by priority (highest first)
// then by prefix length (longest first).
//
// Matching criteria:
//   - Route must be enabled
//   - Route must have the same Type (e.g., SMS)
//   - Route's Prefix must match the message destination (longest prefix wins)
func (m *RouteMatcher) Match(msg *domain.Message, routes []domain.Route) []domain.Route {
	if len(routes) == 0 {
		return nil
	}

	dest := normalizeDestination(msg.Destination)
	var matched []domain.Route

	for _, r := range routes {
		if !r.Enabled {
			continue
		}
		if r.Type != domain.RouteTypeSMS {
			continue
		}
		if !prefixMatches(r.Prefix, dest) {
			continue
		}
		matched = append(matched, r)
	}

	// Sort: highest priority first, then longest prefix first.
	// Stable sort preserves insertion order for equal priorities.
	sortRoutes(matched)
	return matched
}

// prefixMatches checks if the route prefix matches the normalized destination.
// An empty prefix matches everything (catch-all route).
func prefixMatches(prefix, dest string) bool {
	if prefix == "" {
		return true // catch-all
	}
	return strings.HasPrefix(dest, prefix)
}

// normalizeDestination strips common prefixes and whitespace for matching.
// E.g., "+1234567890" becomes "1234567890" for prefix matching against "1".
func normalizeDestination(dest string) string {
	dest = strings.TrimSpace(dest)
	dest = strings.TrimPrefix(dest, "+")
	dest = strings.TrimPrefix(dest, "00")
	return dest
}

// sortRoutes orders routes by priority (desc) then prefix length (desc).
func sortRoutes(routes []domain.Route) {
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			swap := false
			if routes[j].Priority > routes[i].Priority {
				swap = true
			} else if routes[j].Priority == routes[i].Priority &&
				len(routes[j].Prefix) > len(routes[i].Prefix) {
				swap = true
			}
			if swap {
				routes[i], routes[j] = routes[j], routes[i]
			}
		}
	}
}
