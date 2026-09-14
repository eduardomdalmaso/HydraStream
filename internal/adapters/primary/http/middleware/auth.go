package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	jwtSecret     []byte
	jwtSecretOnce sync.Once
)

func getJWTSecret() []byte {
	jwtSecretOnce.Do(func() {
		secret := os.Getenv("JWT_SECRET")
		if len(secret) >= 32 {
			jwtSecret = []byte(secret)
		} else {
			if len(secret) > 0 {
				log.Println("⚠️ [SECURITY WARNING] JWT_SECRET is shorter than 32 characters; generating secure ephemeral 256-bit key")
			} else {
				log.Println("⚠️ [SECURITY WARNING] JWT_SECRET not configured in environment; generating secure ephemeral 256-bit key")
			}
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				log.Fatalf("❌ [FATAL] Failed to initialize secure cryptographic RNG for JWT: %v", err)
			}
			jwtSecret = key
		}
	})
	return jwtSecret
}

// JWTClaims represents standard claims expected in Hydra ecosystem tokens.
type JWTClaims struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

// ValidateJWT verifies HMAC-SHA256 signature and expiration of a JWT string.
func ValidateJWT(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	headerB64, payloadB64, signatureB64 := parts[0], parts[1], parts[2]

	// Verify Signature
	mac := hmac.New(sha256.New, getJWTSecret())
	mac.Write([]byte(headerB64 + "." + payloadB64))
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(signatureB64)
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid token signature")
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("invalid payload structure")
	}

	// Verify expiration
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, errors.New("token expired")
	}

	if claims.TenantID == "" {
		claims.TenantID = "default"
	}
	if claims.Role == "" {
		claims.Role = "viewer"
	}

	return &claims, nil
}

// AuthMiddleware enforces token authentication on protected endpoints.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Check for Service API Key in header
		apiKey := r.Header.Get("X-API-Key")
		expectedAPIKey := os.Getenv("SERVICE_API_KEY")
		if expectedAPIKey != "" && apiKey != "" && hmac.Equal([]byte(apiKey), []byte(expectedAPIKey)) {
			ctx := WithTenantContext(r.Context(), "system")
			ctx = WithUserContext(ctx, "service-account", "superadmin")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 2. Extract Bearer token from Authorization header
		authHeader := r.Header.Get("Authorization")
		tokenStr := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "bearer ")
		} else if qToken := r.URL.Query().Get("token"); qToken != "" {
			tokenStr = qToken // Allow query token for WebRTC / snapshot URLs if needed
		}

		if tokenStr == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication token required","code":"UNAUTHORIZED"}`))
			return
		}

		claims, err := ValidateJWT(tokenStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(fmt.Sprintf(`{"error":"invalid token: %s","code":"INVALID_TOKEN"}`, err.Error())))
			return
		}

		// Inject tenant and user info into request context
		ctx := WithTenantContext(r.Context(), claims.TenantID)
		ctx = WithUserContext(ctx, claims.UserID, claims.Role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole verifies that the authenticated user possesses one of the required roles.
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole := GetUserRole(r.Context())
			if userRole == "superadmin" {
				next.ServeHTTP(w, r)
				return
			}

			for _, role := range allowedRoles {
				if strings.EqualFold(userRole, role) {
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":"forbidden: insufficient role permissions","code":"FORBIDDEN"}`))
		})
	}
}
