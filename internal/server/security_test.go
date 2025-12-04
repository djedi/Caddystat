package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Test that security headers are set on various endpoints
	endpoints := []string{"/health", "/robots.txt", "/api/auth/check"}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()

			srv.ServeHTTP(w, req)

			// Check Content-Security-Policy
			csp := w.Header().Get("Content-Security-Policy")
			if csp == "" {
				t.Error("Content-Security-Policy header not set")
			}
			if !strings.Contains(csp, "default-src 'self'") {
				t.Errorf("CSP missing default-src 'self', got: %s", csp)
			}
			if !strings.Contains(csp, "script-src") {
				t.Errorf("CSP missing script-src directive, got: %s", csp)
			}
			if !strings.Contains(csp, "frame-ancestors 'none'") {
				t.Errorf("CSP missing frame-ancestors 'none', got: %s", csp)
			}

			// Check X-Content-Type-Options
			xcto := w.Header().Get("X-Content-Type-Options")
			if xcto != "nosniff" {
				t.Errorf("expected X-Content-Type-Options 'nosniff', got %q", xcto)
			}

			// Check X-Frame-Options
			xfo := w.Header().Get("X-Frame-Options")
			if xfo != "DENY" {
				t.Errorf("expected X-Frame-Options 'DENY', got %q", xfo)
			}

			// Check Referrer-Policy
			rp := w.Header().Get("Referrer-Policy")
			if rp != "strict-origin-when-cross-origin" {
				t.Errorf("expected Referrer-Policy 'strict-origin-when-cross-origin', got %q", rp)
			}
		})
	}
}

func TestCSRFCookieIsSet(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Check that CSRF cookie is set
	cookies := w.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}

	if csrfCookie == nil {
		t.Fatal("CSRF cookie not set")
	}
	if csrfCookie.Value == "" {
		t.Error("CSRF cookie value is empty")
	}
	if csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", csrfCookie.SameSite)
	}
}

func TestCSRFCookieReused(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// First request - get the CSRF cookie
	req1 := httptest.NewRequest(http.MethodGet, "/health", nil)
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, req1)

	cookies := w1.Result().Cookies()
	var firstToken string
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			firstToken = c.Value
			break
		}
	}

	// Second request with existing cookie
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: firstToken})
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	// Should not set a new cookie if one already exists
	cookies2 := w2.Result().Cookies()
	for _, c := range cookies2 {
		if c.Name == csrfCookieName && c.Value != firstToken {
			t.Error("CSRF cookie was regenerated when it shouldn't be")
		}
	}
}

func TestCSRFProtectionRequiresToken(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// POST request without CSRF token should be rejected
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"test","password":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "CSRF") {
		t.Errorf("expected error message about CSRF, got: %s", body)
	}
}

func TestCSRFProtectionWithValidToken(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// First, get a CSRF token
	req1 := httptest.NewRequest(http.MethodGet, "/health", nil)
	w1 := httptest.NewRecorder()
	srv.ServeHTTP(w1, req1)

	var csrfToken string
	for _, c := range w1.Result().Cookies() {
		if c.Name == csrfCookieName {
			csrfToken = c.Value
			break
		}
	}

	// POST request with valid CSRF token
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"test","password":"test"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(csrfHeaderName, csrfToken)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	w2 := httptest.NewRecorder()

	srv.ServeHTTP(w2, req2)

	// Should not be forbidden (may be unauthorized due to wrong credentials, but not CSRF error)
	if w2.Code == http.StatusForbidden {
		body := w2.Body.String()
		if strings.Contains(body, "CSRF") {
			t.Errorf("request was rejected due to CSRF when token was valid")
		}
	}
}

func TestCSRFProtectionTokenMismatch(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// POST request with mismatched CSRF token
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"test","password":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, "wrong-token")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "correct-token"})
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d for mismatched token, got %d", http.StatusForbidden, w.Code)
	}
}

func TestCSRFProtectionGETRequestsAllowed(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// GET request should not require CSRF token
	req := httptest.NewRequest(http.MethodGet, "/api/auth/check", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Should not be forbidden (GET doesn't require CSRF)
	if w.Code == http.StatusForbidden {
		t.Errorf("GET request was rejected, expected it to be allowed")
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	// Generate multiple tokens and ensure they're unique
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateCSRFToken()
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}
		if token == "" {
			t.Error("generated empty token")
		}
		if tokens[token] {
			t.Error("generated duplicate token")
		}
		tokens[token] = true
	}
}

func TestValidateCSRFTokenConstantTime(t *testing.T) {
	// This is a basic sanity check - proper timing tests are complex
	// The implementation uses subtle.ConstantTimeCompare

	// Create request with matching tokens
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	token := "test-token-12345"
	req.Header.Set(csrfHeaderName, token)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})

	if !validateCSRFToken(req) {
		t.Error("expected matching tokens to validate")
	}

	// Create request with non-matching tokens
	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.Header.Set(csrfHeaderName, "wrong-token")
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})

	if validateCSRFToken(req2) {
		t.Error("expected non-matching tokens to fail validation")
	}
}

func TestCSRFCookieSecureFlag(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	t.Run("HTTP request gets non-secure cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		var csrfCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == csrfCookieName {
				csrfCookie = c
				break
			}
		}

		if csrfCookie == nil {
			t.Fatal("CSRF cookie not set")
		}
		if csrfCookie.Secure {
			t.Error("expected Secure=false for HTTP request")
		}
	})

	t.Run("HTTPS via X-Forwarded-Proto gets secure cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		var csrfCookie *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == csrfCookieName {
				csrfCookie = c
				break
			}
		}

		if csrfCookie == nil {
			t.Fatal("CSRF cookie not set")
		}
		if !csrfCookie.Secure {
			t.Error("expected Secure=true for HTTPS request via proxy")
		}
	})
}
