package httpapi

import (
	"net/http"
	"strings"
	"time"
)

const (
	AccessTokenCookieName  = "access_token"
	RefreshTokenCookieName = "refresh_token"
)

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(r.Header.Get("CF-Visitor"), `{"scheme":"https"}`)
}

func setAuthCookies(w http.ResponseWriter, r *http.Request, accessToken string, accessExpires time.Time, refreshToken string, refreshExpires time.Time) {
	secure := isSecureRequest(r)
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		Expires:  accessExpires,
		MaxAge:   int(time.Until(accessExpires).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		Expires:  refreshExpires,
		MaxAge:   int(time.Until(refreshExpires).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := isSecureRequest(r)
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}
