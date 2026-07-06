package pipeline

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// GSM-7 character set detection.
// Basic GSM 03.38 7-bit default alphabet characters.
// Characters outside this set require UCS-2 encoding.
const gsm7Chars = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ\x1bÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// Basic GSM-7 extension table characters (require escape prefix 0x1B).
var gsm7ExtChars = map[rune]bool{
	'\f': true, // form feed
	'^':  true,
	'{':  true,
	'}':  true,
	'\\': true,
	'[':  true,
	'~':  true,
	']':  true,
	'|':  true,
	'€':  true,
}

const (
	// GSM7MaxLen is the maximum single-part length for GSM-7 encoding.
	GSM7MaxLen = 160

	// GSM7ConcatLen is the per-part length for concatenated GSM-7 messages.
	GSM7ConcatLen = 153

	// UCS2MaxLen is the maximum single-part length for UCS-2 encoding.
	UCS2MaxLen = 70

	// UCS2ConcatLen is the per-part length for concatenated UCS-2 messages.
	UCS2ConcatLen = 67
)

var (
	// ErrInvalidEncoding is returned when encoding detection fails.
	ErrInvalidEncoding = fmt.Errorf("invalid encoding")

	// ErrInvalidDestination is returned when the destination number is invalid.
	ErrInvalidDestination = fmt.Errorf("invalid destination number")
)

// PrepareStage normalizes message fields and computes derived values.
// Responsibilities:
//   - Normalize phone numbers to canonical format
//   - Detect encoding (GSM-7 or UCS-2)
//   - Calculate SMS part count
//   - Fill SendRequest on PipelineState
//
// No I/O, no routing, no pricing, no database access.
type PrepareStage struct{}

// NewPrepareStage creates a new PrepareStage.
func NewPrepareStage() *PrepareStage {
	return &PrepareStage{}
}

// Name returns the stage name for logging and metrics.
func (s *PrepareStage) Name() string {
	return "prepare"
}

// Process normalizes the message and fills derived fields.
func (s *PrepareStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	msg := state.Message

	// 1. Normalize destination to E.164-like format (strip non-digits, ensure leading +)
	dest := normalizePhone(msg.Destination)
	if dest == "" {
		return nil, fmt.Errorf("%w: %q after normalization", ErrInvalidDestination, msg.Destination)
	}
	msg.Destination = dest

	// 2. Detect encoding based on text content
	encoding, err := detectEncoding(msg.Text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEncoding, err)
	}

	// 3. Calculate number of SMS parts
	parts := calculateParts(msg.Text, encoding)

	// 4. Update message fields
	msg.Encoding = domain.Encoding(encoding)
	msg.Parts = parts

	// 5. Fill SendRequest for downstream stages
	state.SendRequest = &SendRequest{
		MessageID:   msg.ID,
		Source:      msg.Source,
		Destination: msg.Destination,
		Text:        msg.Text,
		Encoding:    encoding,
		Parts:       parts,
	}

	return state, nil
}

// normalizePhone strips non-digit characters and ensures E.164-like format.
// Examples:
//
//	"+1 (234) 567-8900" → "+12345678900"
//	"1234567890"        → "+1234567890" (if no +, assumes leading digit)
func normalizePhone(phone string) string {
	// Strip all non-digit, non-plus characters
	var b strings.Builder
	b.Grow(len(phone) + 1)

	hasPlus := false
	for i := 0; i < len(phone); i++ {
		c := phone[i]
		if c == '+' && !hasPlus {
			hasPlus = true
			b.WriteByte('+')
		} else if c >= '0' && c <= '9' {
			b.WriteByte(c)
		}
	}

	result := b.String()
	if result == "" {
		return ""
	}

	// Ensure leading +
	if result[0] != '+' {
		result = "+" + result
	}

	return result
}

// detectEncoding determines whether a message can be sent as GSM-7
// or requires UCS-2 encoding.
func detectEncoding(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty text")
	}

	for _, r := range text {
		if !isGSM7(r) {
			return "UCS2", nil
		}
	}
	return "GSM7", nil
}

// isGSM7 checks if a rune belongs to the GSM 03.38 7-bit alphabet
// (including extension characters).
func isGSM7(r rune) bool {
	// Check extension table first
	if gsm7ExtChars[r] {
		return true
	}
	// Check basic character set
	for _, gsmRune := range gsm7Chars {
		if r == gsmRune {
			return true
		}
	}
	return false
}

// calculateParts computes the number of SMS parts needed.
func calculateParts(text string, encoding string) int {
	if text == "" {
		return 1
	}

	charCount := utf8.RuneCountInString(text)

	var maxLen, concatLen int
	switch encoding {
	case "GSM7":
		maxLen = GSM7MaxLen
		concatLen = GSM7ConcatLen
	case "UCS2":
		maxLen = UCS2MaxLen
		concatLen = UCS2ConcatLen
	default:
		return 1
	}

	if charCount <= maxLen {
		return 1
	}

	// Calculate parts for concatenated SMS
	parts := charCount / concatLen
	if charCount%concatLen != 0 {
		parts++
	}
	return parts
}
