package httpapi

import (
	"net"
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

func getCookieDomain(r *http.Request, configuredDomain string) string {
	if configuredDomain != "" {
		return configuredDomain
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if strings.HasSuffix(host, "nihaowu.pl") {
		return "nihaowu.pl"
	}
	return ""
}

func setAuthCookies(w http.ResponseWriter, r *http.Request, cookieDomain string, accessToken string, accessExpires time.Time, refreshToken string, refreshExpires time.Time) {
	secure := isSecureRequest(r)
	domain := getCookieDomain(r, cookieDomain)
	sameSite := http.SameSiteLaxMode
	if secure && domain == "" {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		Domain:   domain,
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
		Domain:   domain,
		Expires:  refreshExpires,
		MaxAge:   int(time.Until(refreshExpires).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request, cookieDomain string) {
	secure := isSecureRequest(r)
	domain := getCookieDomain(r, cookieDomain)
	sameSite := http.SameSiteLaxMode
	if secure && domain == "" {
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		Domain:   domain,
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
		Domain:   domain,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
}
