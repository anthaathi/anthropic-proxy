package model

import (
	"strings"
)

// MatchAlias checks if a requested model name matches an alias pattern
// Supports wildcard matching with * character
// Examples:
//   - "opus*" matches "opus", "opus-4", "opus-test"
//   - "opus" matches only "opus" exactly
//   - "*opus" matches "test-opus", "opus"
func MatchAlias(aliasPattern, requestedName string) bool {
	// No wildcard - exact match required
	if !strings.Contains(aliasPattern, "*") {
		return aliasPattern == requestedName
	}

	// Split by * to get pattern parts
	parts := strings.Split(aliasPattern, "*")

	// If pattern starts with *, skip the prefix check
	// If pattern ends with *, skip the suffix check

	// Simple case: single * at the end (prefix match)
	if strings.HasSuffix(aliasPattern, "*") && len(parts) == 2 && parts[1] == "" {
		return strings.HasPrefix(requestedName, parts[0])
	}

	// Simple case: single * at the start (suffix match)
	if strings.HasPrefix(aliasPattern, "*") && len(parts) == 2 && parts[0] == "" {
		return strings.HasSuffix(requestedName, parts[1])
	}

	// Complex case: * in the middle or multiple *
	// Convert to a simple contains-based matcher
	return matchWildcard(aliasPattern, requestedName)
}

// matchWildcard performs wildcard matching for complex patterns
func matchWildcard(pattern, text string) bool {
	// Split pattern by *
	parts := strings.Split(pattern, "*")

	if len(parts) == 1 {
		// No wildcard
		return pattern == text
	}

	// Check first part (prefix)
	if parts[0] != "" && !strings.HasPrefix(text, parts[0]) {
		return false
	}
	text = strings.TrimPrefix(text, parts[0])

	// Check last part (suffix)
	lastIdx := len(parts) - 1
	if parts[lastIdx] != "" && !strings.HasSuffix(text, parts[lastIdx]) {
		return false
	}
	text = strings.TrimSuffix(text, parts[lastIdx])

	// Check middle parts (must appear in order)
	for i := 1; i < lastIdx; i++ {
		if parts[i] == "" {
			continue
		}
		idx := strings.Index(text, parts[i])
		if idx == -1 {
			return false
		}
		text = text[idx+len(parts[i]):]
	}

	return true
}
