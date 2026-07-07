package pipeline

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// GSM-7 character set — O(1) lookup map.
// Basic GSM 03.38 7-bit default alphabet characters.
var gsm7Chars = map[rune]struct{}{
	'@': {}, '£': {}, '$': {}, '¥': {}, 'è': {}, 'é': {}, 'ù': {}, 'ì': {}, 'ò': {}, 'Ç': {},
	'\n': {}, 'Ø': {}, 'ø': {}, '\r': {}, 'Å': {}, 'å': {}, 'Δ': {}, '_': {}, 'Φ': {}, 'Γ': {},
	'Λ': {}, 'Ω': {}, 'Π': {}, 'Ψ': {}, 'Σ': {}, 'Θ': {}, 'Ξ': {}, '\x1b': {}, 'Æ': {}, 'æ': {},
	'ß': {}, 'É': {}, ' ': {}, '!': {}, '"': {}, '#': {}, '¤': {}, '%': {}, '&': {}, '\'': {},
	'(': {}, ')': {}, '*': {}, '+': {}, ',': {}, '-': {}, '.': {}, '/': {}, '0': {}, '1': {},
	'2': {}, '3': {}, '4': {}, '5': {}, '6': {}, '7': {}, '8': {}, '9': {}, ':': {}, ';': {},
	'<': {}, '=': {}, '>': {}, '?': {}, '¡': {}, 'A': {}, 'B': {}, 'C': {}, 'D': {}, 'E': {},
	'F': {}, 'G': {}, 'H': {}, 'I': {}, 'J': {}, 'K': {}, 'L': {}, 'M': {}, 'N': {}, 'O': {},
	'P': {}, 'Q': {}, 'R': {}, 'S': {}, 'T': {}, 'U': {}, 'V': {}, 'W': {}, 'X': {}, 'Y': {},
	'Z': {}, 'Ä': {}, 'Ö': {}, 'Ñ': {}, 'Ü': {}, '§': {}, '¿': {}, 'a': {}, 'b': {}, 'c': {},
	'd': {}, 'e': {}, 'f': {}, 'g': {}, 'h': {}, 'i': {}, 'j': {}, 'k': {}, 'l': {}, 'm': {},
	'n': {}, 'o': {}, 'p': {}, 'q': {}, 'r': {}, 's': {}, 't': {}, 'u': {}, 'v': {}, 'w': {},
	'x': {}, 'y': {}, 'z': {}, 'ä': {}, 'ö': {}, 'ñ': {}, 'ü': {}, 'à': {},
}

// gsm7ExtensionChars — GSM-7 extension characters that count as 2 chars.
// These require the escape prefix (0x1B) before the actual character encoding.
var gsm7ExtensionChars = map[rune]struct{}{
	'\f': {}, // form feed
	'^':  {},
	'{':  {},
	'}':  {},
	'\\': {},
	'[':  {},
	'~':  {},
	']':  {},
	'|':  {},
	'€':  {},
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
//   - Fill PreparedMessage on PipelineState
//
// It does NOT mutate domain.Message — all derived values go into PreparedMessage.
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

// Process normalizes the message and fills derived fields into PreparedMessage.
// The domain.Message is NOT mutated — all derived values go into PreparedMessage only.
func (s *PrepareStage) Process(ctx context.Context, state *PipelineState) (*PipelineState, error) {
	msg := state.Message

	// 1. Normalize destination to E.164-like format (strip non-digits, ensure +).
	// The normalized version goes into PreparedMessage, NOT msg.Destination.
	dest := normalizePhone(msg.Destination)
	if dest == "" {
		return nil, fmt.Errorf("%w: %q after normalization", ErrInvalidDestination, msg.Destination)
	}

	// 2. Detect encoding based on text content
	encoding, err := detectEncoding(msg.Text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEncoding, err)
	}

	// 3. Calculate number of SMS parts (GSM-7 extension chars count as 2)
	parts := calculateParts(msg.Text, encoding)

	// 4. Fill PreparedMessage with pipeline-local preparation results.
	//    Only derived values that cannot live on domain.Message go here.
	//    domain.SendRequest carries them (Message + Destination/Encoding/Parts)
	//    to the sender in the next stage.
	state.Prepared = PreparedMessage{
		Destination: dest,
		Encoding:    encoding,
		Parts:       parts,
	}

	return state, nil
}

// normalizePhone strips non-digit characters and ensures leading +.
// This is formatting, not E.164 validation. Full validation is a separate stage.
//
//	"+1 (234) 567-8900" → "+12345678900"
//	"1234567890"        → "+1234567890"
func normalizePhone(phone string) string {
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
	if _, ok := gsm7ExtensionChars[r]; ok {
		return true
	}
	_, ok := gsm7Chars[r]
	return ok
}

// gsm7CharLength returns the character length in GSM-7 encoding.
// Extension chars count as 2 (escape prefix + actual char).
// Basic chars count as 1.
func gsm7CharLength(r rune) int {
	if _, ok := gsm7ExtensionChars[r]; ok {
		return 2
	}
	return 1
}

// calculateParts computes the number of SMS parts needed.
// For GSM-7, extension characters count as 2 chars each.
func calculateParts(text string, encoding string) int {
	if text == "" {
		return 1
	}

	var totalChars int
	switch encoding {
	case "GSM7":
		for _, r := range text {
			totalChars += gsm7CharLength(r)
		}
	default:
		totalChars = utf8.RuneCountInString(text)
	}

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

	if totalChars <= maxLen {
		return 1
	}

	parts := totalChars / concatLen
	if totalChars%concatLen != 0 {
		parts++
	}
	return parts
}
