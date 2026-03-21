package structured

import (
	"errors"
	"strings"
)

// ExtractJSON tries to recover a valid JSON object/array from model output.
// It accepts pure JSON or mixed text with embedded JSON.
func ExtractJSON(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", errors.New("empty response")
	}

	if isValidJSON(trimmed) {
		return trimmed, nil
	}

	for i := 0; i < len(text); i++ {
		if text[i] != '{' && text[i] != '[' {
			continue
		}

		end, ok := findBalancedJSON(text, i)
		if !ok {
			continue
		}

		candidate := strings.TrimSpace(text[i:end])
		if isValidJSON(candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("no valid JSON object or array found")
}

func findBalancedJSON(s string, start int) (int, bool) {
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false

	for i := start; i < len(s); i++ {
		ch := s[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 {
				return 0, false
			}
			last := stack[len(stack)-1]
			if (ch == '}' && last != '{') || (ch == ']' && last != '[') {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}

	return 0, false
}
