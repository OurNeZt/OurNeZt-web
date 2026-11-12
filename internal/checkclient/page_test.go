package checkclient

import (
	"strings"
	"testing"
)

func TestNormalizeLoginPagePreservesAccountDifferences(t *testing.T) {
	page := `<input type="hidden" name="csrf_token" value="first"><script nonce="first">run()</script><p>Invalid credentials</p>`
	other := strings.ReplaceAll(page, "first", "second")
	if NormalizeLoginPage(page) != NormalizeLoginPage(other) {
		t.Fatal("random per-response values were not normalized")
	}
	for _, changed := range []string{
		strings.ReplaceAll(other, "Invalid credentials", "Account disabled"),
		strings.ReplaceAll(other, "run()", "different()"),
	} {
		if NormalizeLoginPage(page) == NormalizeLoginPage(changed) {
			t.Fatal("meaningful login page difference was hidden")
		}
	}
}
