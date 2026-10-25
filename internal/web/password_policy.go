package web

import (
	"errors"
	"strings"
	"unicode"
)

const passwordWhitespaceMessage = "new passwords must not contain spaces or other whitespace"

// Core enforces the full policy. Reject whitespace in raw form values here so
// the browser receives a clear error and no password is silently trimmed.
func validateNewPasswordWhitespace(password string) error {
	if strings.IndexFunc(password, unicode.IsSpace) >= 0 {
		return errors.New(passwordWhitespaceMessage)
	}
	return nil
}
