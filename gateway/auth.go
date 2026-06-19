package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// -------------------------------------------------------------------
// Authentication — JWT-based login with API key support
//
// Two auth methods:
//   1. Login (POST /api/auth/login) → JWT token (24h expiry)
//   2. API key (X-API-Key header) → for SDK/automation access
//
// Protected routes require Authorization: Bearer <token> or X-API-Key.
// WebSocket endpoints (/ws, /siprec) and /healthz are public.
// -------------------------------------------------------------------

type AuthHandler struct {
	db        *sql.DB
	jwtSecret []byte
}

type UserClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"` // admin, agent, viewer
	Exp      int64  `json:"exp"`
}

func NewAuthHandler(db *sql.DB, secret string) *AuthHandler {
	if secret == "" {
		secret = "voiceagent-default-secret-change-in-production"
	}
	ah := &AuthHandler{
		db:        db,
		jwtSecret: []byte(secret),
	}
	if db != nil {
		ah.initDB()
		ah.seedDefaultUser()
	}
	return ah
}

func (ah *AuthHandler) initDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ah.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT DEFAULT 'admin',
			api_key TEXT UNIQUE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
}

func (ah *AuthHandler) seedDefaultUser() {
	var count int
	ah.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count == 0 {
		hash := hashPassword("admin")
		apiKey := generateAPIKey("admin")
		ah.db.Exec("INSERT INTO users (username, password_hash, role, api_key) VALUES ($1, $2, $3, $4)",
			"admin", hash, "admin", apiKey)
		slog.Info("default user created", "username", "admin", "password", "admin", "api_key", apiKey)
	}
}

func (ah *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", ah.handleLogin)
	mux.HandleFunc("/api/auth/me", ah.handleMe)
	mux.HandleFunc("/api/auth/users", ah.handleUsers)
	mux.HandleFunc("/api/auth/apikey", ah.handleAPIKey)
}

// -------------------------------------------------------------------
// Login endpoint
// -------------------------------------------------------------------

func (ah *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
		return
	}

	// Check credentials
	var userID, storedHash, role string
	var err error

	if ah.db != nil {
		err = ah.db.QueryRow("SELECT id, password_hash, role FROM users WHERE username = $1", req.Username).
			Scan(&userID, &storedHash, &role)
	} else {
		// Fallback: hardcoded admin/admin
		if req.Username == "admin" && req.Password == "admin" {
			userID = "default"
			storedHash = hashPassword("admin")
			role = "admin"
		} else {
			err = fmt.Errorf("invalid credentials")
		}
	}

	if err != nil || hashPassword(req.Password) != storedHash {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Generate JWT
	token := ah.generateToken(UserClaims{
		UserID:   userID,
		Username: req.Username,
		Role:     role,
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"token":    token,
		"username": req.Username,
		"role":     role,
	})
}

func (ah *AuthHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	claims := getClaimsFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, claims)
}

func (ah *AuthHandler) handleUsers(w http.ResponseWriter, r *http.Request) {
	claims := getClaimsFromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin only"})
		return
	}

	switch r.Method {
	case "GET":
		if ah.db == nil {
			writeJSON(w, http.StatusOK, []map[string]string{{"username": "admin", "role": "admin"}})
			return
		}
		rows, _ := ah.db.Query("SELECT id, username, role, created_at FROM users ORDER BY created_at")
		defer rows.Close()
		var users []map[string]any
		for rows.Next() {
			var id, username, role string
			var createdAt time.Time
			rows.Scan(&id, &username, &role, &createdAt)
			users = append(users, map[string]any{"id": id, "username": username, "role": role, "created_at": createdAt})
		}
		if users == nil {
			users = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, users)

	case "POST":
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Role == "" {
			req.Role = "agent"
		}
		if ah.db != nil {
			apiKey := generateAPIKey(req.Username)
			ah.db.Exec("INSERT INTO users (username, password_hash, role, api_key) VALUES ($1,$2,$3,$4)",
				req.Username, hashPassword(req.Password), req.Role, apiKey)
			writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "api_key": apiKey})
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"status": "created (no db)"})
		}

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (ah *AuthHandler) handleAPIKey(w http.ResponseWriter, r *http.Request) {
	claims := getClaimsFromContext(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if ah.db == nil {
		writeJSON(w, http.StatusOK, map[string]string{"api_key": "no-db-mode"})
		return
	}
	var apiKey string
	ah.db.QueryRow("SELECT api_key FROM users WHERE username = $1", claims.Username).Scan(&apiKey)
	writeJSON(w, http.StatusOK, map[string]string{"api_key": apiKey})
}

// -------------------------------------------------------------------
// JWT token generation and validation
// -------------------------------------------------------------------

func (ah *AuthHandler) generateToken(claims UserClaims) string {
	payload, _ := json.Marshal(claims)
	signature := ah.sign(payload)
	return hex.EncodeToString(payload) + "." + signature
}

func (ah *AuthHandler) validateToken(token string) *UserClaims {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	payload, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil
	}

	expectedSig := ah.sign(payload)
	if parts[1] != expectedSig {
		return nil
	}

	var claims UserClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}

	if time.Now().Unix() > claims.Exp {
		return nil
	}

	return &claims
}

func (ah *AuthHandler) sign(data []byte) string {
	h := hmac.New(sha256.New, ah.jwtSecret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// -------------------------------------------------------------------
// Auth middleware
// -------------------------------------------------------------------

func (ah *AuthHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints — no auth required
		if path == "/healthz" || path == "/metrics" || path == "/api/auth/login" ||
			path == "/ws" || path == "/siprec" || path == "/siprec/events" ||
			strings.HasPrefix(path, "/_next") || path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key first
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "" && ah.db != nil {
			var username, role string
			err := ah.db.QueryRow("SELECT username, role FROM users WHERE api_key = $1", apiKey).Scan(&username, &role)
			if err == nil {
				claims := &UserClaims{Username: username, Role: role}
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Check Bearer token (header) or query param (for SSE/EventSource)
		authHeader := r.Header.Get("Authorization")
		token := ""
		if authHeader != "" {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if qToken := r.URL.Query().Get("token"); qToken != "" {
			token = qToken
		}

		if token == "" {
			if envOr("AUTH_ENABLED", "") != "true" {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		claims := ah.validateToken(token)
		if claims == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type contextKey string

const claimsKey contextKey = "user_claims"

func getClaimsFromContext(ctx context.Context) *UserClaims {
	claims, _ := ctx.Value(claimsKey).(*UserClaims)
	return claims
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func hashPassword(password string) string {
	h := sha256.Sum256([]byte("voiceagent:" + password))
	return hex.EncodeToString(h[:])
}

func generateAPIKey(username string) string {
	data := fmt.Sprintf("%s:%d", username, time.Now().UnixNano())
	h := sha256.Sum256([]byte(data))
	return "va_" + hex.EncodeToString(h[:16])
}
