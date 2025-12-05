package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// Content-Security-Policy header value.
// This policy allows:
// - Scripts from self, cdn.jsdelivr.net (Alpine.js), unsafe-inline and unsafe-eval (required by Alpine.js)
// - Styles from self, fonts.googleapis.com, and unsafe-inline (for Alpine.js dynamic styles)
// - Fonts from fonts.gstatic.com
// - Images from self and data: URIs (for favicon)
// - Connect to self only (for API/SSE)
// Note: Alpine.js requires 'unsafe-eval' to evaluate x-data, x-show, @click expressions.
// The CSP build of Alpine.js (alpine-csp) could be used instead to avoid unsafe-eval,
// but would require significant refactoring of the frontend.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'; " +
	"base-uri 'self'"

// csrfTokenLength is the length of CSRF tokens in bytes (before base64 encoding).
const csrfTokenLength = 32

// csrfCookieName is the name of the cookie storing the CSRF token.
const csrfCookieName = "caddystat_csrf"

// csrfHeaderName is the name of the header that must contain the CSRF token.
const csrfHeaderName = "X-CSRF-Token"

// setSecurityHeaders adds security-related headers to the response.
func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

// generateCSRFToken creates a new random CSRF token.
func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ensureCSRFCookie ensures that a CSRF cookie is set on the response.
// Returns the token value (either from existing cookie or newly generated).
func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	// Check if cookie already exists
	if cookie, err := r.Cookie(csrfCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}

	// Generate new token
	token, err := generateCSRFToken()
	if err != nil {
		return "", err
	}

	// Detect if request is over HTTPS (direct TLS or via proxy)
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JavaScript needs to read this
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure,
	})

	return token, nil
}

// validateCSRFToken validates that the CSRF token from the header matches the cookie.
func validateCSRFToken(r *http.Request) bool {
	// Get token from cookie
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	// Get token from header
	headerToken := r.Header.Get(csrfHeaderName)
	if headerToken == "" {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) == 1
}

// requireCSRF is middleware that validates CSRF tokens for state-changing requests.
func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only validate for methods that change state
		method := strings.ToUpper(r.Method)
		if method == "POST" || method == "PUT" || method == "DELETE" || method == "PATCH" {
			if !validateCSRFToken(r) {
				writeErrorWithCode(w, http.StatusForbidden, "invalid or missing CSRF token", "CSRF_INVALID")
				return
			}
		}
		next(w, r)
	}
}
