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

// Display only known policy reasons. Unexpected Core validation text must not
// expose account details or credentials in a redirect or rendered page.
func passwordValidationError(err error) error {
	reason := strings.TrimPrefix(grpcMessage(err), "invalid argument: ")
	switch reason {
	case "password must not contain whitespace", "password must not be blank or whitespace-only":
		return errors.New(passwordWhitespaceMessage)
	case "password must contain at least 15 characters":
		return errors.New("new password must contain at least 15 characters")
	case "password must not exceed 1024 bytes", "new_password must not exceed 1024 bytes":
		return errors.New("new password must not exceed 1,024 UTF-8 bytes")
	case "current_password must not exceed 1024 bytes":
		return errors.New("current password must not exceed 1,024 UTF-8 bytes")
	case "password is too common; choose a less predictable password":
		return errors.New("new password is too common; choose a less predictable password")
	case "password must be valid UTF-8":
		return errors.New("new password must contain valid UTF-8 text")
	default:
		return errors.New("password validation failed; check the password requirements")
	}
}
