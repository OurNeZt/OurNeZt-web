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
		Title: title,
		User:  user,
		Error: strings.TrimSpace(c.Query("error")),
		Flash: strings.TrimSpace(c.Query("flash")),
		Data:  payload,
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

func parseInt64(raw string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return v
}

func centsString(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s$%d.%02d", sign, value/100, value%100)
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
