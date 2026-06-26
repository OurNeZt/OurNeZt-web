package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	ourneztv1 "github.com/OurNeZt/ournezt-web/internal/gen/proto/ournezt/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func (a *App) render(c *gin.Context, templateName, title string, payload any) {
	var user *CurrentUser
	if value, ok := c.Get(userContextKey); ok {
		if current, castOK := value.(*CurrentUser); castOK {
			user = current
		}
	}

	c.HTML(http.StatusOK, templateName, ViewData{
		Title:             title,
		User:              user,
		Error:             strings.TrimSpace(c.Query("error")),
		Flash:             strings.TrimSpace(c.Query("flash")),
		MaintenanceNotice: a.maintenanceNotice(),
		Data:              payload,
	})
}

func (a *App) grpcContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if token, ok := c.Get(tokenContextKey); ok {
		if t, castOK := token.(string); castOK && t != "" {
			return metadata.AppendToOutgoingContext(ctx, "x-session-token", t)
		}
	}
	return ctx
}

func userFromContext(c *gin.Context) *CurrentUser {
	value, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, ok := value.(*CurrentUser)
	if !ok {
		return nil
	}
	return user
}

func sessionCookie(name, value string, maxAge time.Duration, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(maxAge.Seconds()),
	}
}

func clearSessionCookie(name string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func grpcMessage(err error) string {
	if err == nil {
		return ""
	}
	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}
	return st.Message()
}

func grpcCode(err error) codes.Code {
	st, ok := status.FromError(err)
	if !ok {
		return codes.Unknown
	}
	return st.Code()
}

func parseInt32(raw string) int32 {
	v, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	return int32(v)
}

func parseMoneyCents(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) == 0 {
		return 0
	}

	wholePart := strings.TrimSpace(parts[0])
	if wholePart == "" {
		wholePart = "0"
	}
	whole, wholeErr := strconv.ParseInt(wholePart, 10, 64)
	if wholeErr != nil {
		return 0
	}

	frac := int64(0)
	if len(parts) > 1 {
		fracPart := strings.TrimSpace(parts[1])
		if len(fracPart) > 2 {
			fracPart = fracPart[:2]
		}
		for len(fracPart) < 2 {
			fracPart += "0"
		}
		if fracPart != "" {
			fracParsed, fracErr := strconv.ParseInt(fracPart, 10, 64)
			if fracErr != nil {
				return 0
			}
			frac = fracParsed
		}
	}

	total := whole*100 + frac
	if neg {
		total = -total
	}
	return total
}

func centsInputString(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func parsePercentBps(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	}

	parts := strings.SplitN(s, ".", 3)
	wholePart := "0"
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		wholePart = strings.TrimSpace(parts[0])
	}

	whole, wholeErr := strconv.ParseInt(wholePart, 10, 64)
	if wholeErr != nil {
		return 0
	}

	frac := int64(0)
	if len(parts) > 1 {
		fracPart := strings.TrimSpace(parts[1])
		if len(fracPart) > 2 {
			fracPart = fracPart[:2]
		}
		for len(fracPart) < 2 {
			fracPart += "0"
		}
		if fracPart != "" {
			fracParsed, fracErr := strconv.ParseInt(fracPart, 10, 64)
			if fracErr != nil {
				return 0
			}
			frac = fracParsed
		}
	}

	total := whole*100 + frac
	if neg {
		total = -total
	}
	return total
}

func bpsInputString(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func eqFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func centsString(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	dollars := value / 100
	cents := value % 100
	return fmt.Sprintf("%s$%s.%02d", sign, formatThousands(dollars), cents)
}

func userFromProto(u *ourneztv1.User) *CurrentUser {
	if u == nil {
		return nil
	}
	return &CurrentUser{
		ID:                 u.GetId(),
		Email:              u.GetEmail(),
		DisplayName:        u.GetDisplayName(),
		Role:               u.GetRole(),
		MustChangePassword: u.GetMustChangePassword(),
	}
}

func urlQuerySafe(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func normalizeLookup(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func housingRatingLabel(value string) string {
	switch normalizeLookup(value) {
	case "comfortable":
		return "Comfortable"
	case "manageable":
		return "Manageable"
	case "tight":
		return "Tight"
	case "risky":
		return "Risky"
	case "not_recommended":
		return "Not Recommended"
	default:
		return strings.TrimSpace(value)
	}
}

func isISODate(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", v)
	return err == nil
}

func formatThousands(value int64) string {
	if value == 0 {
		return "0"
	}
	raw := strconv.FormatInt(value, 10)
	n := len(raw)
	if n <= 3 {
		return raw
	}

	head := n % 3
	if head == 0 {
		head = 3
	}

	var b strings.Builder
	b.Grow(n + (n-1)/3)
	b.WriteString(raw[:head])
	for i := head; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(raw[i : i+3])
	}
	return b.String()
}

func normalizeOptionalUUID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if isCanonicalUUID(trimmed) {
		return strings.ToLower(trimmed)
	}
	return ""
}

func isCanonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, ch := range value {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !isHexRune(ch) {
				return false
			}
		}
	}
	return true
}

func isHexRune(ch rune) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}
