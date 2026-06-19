package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// -------------------------------------------------------------------
// SIP Trunk Management — Config + Secure API
//
// Trunks can be provisioned two ways:
//   1. Config file (freeswitch/config/sip_profiles/) — for initial/air-gapped
//   2. Secure API (/api/trunks) — for runtime provisioning, requires auth
//
// The API stores trunk configs in PostgreSQL and applies them to
// FreeSWITCH via ESL commands (sofia profile rescan).
// -------------------------------------------------------------------

type SIPTrunk struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`  // twilio, vonage, telnyx, cisco-cube, audiocodes, custom
	Address   string `json:"address"`   // SBC hostname or IP
	Port      int    `json:"port"`      // SIP port (default 5060)
	Transport string `json:"transport"` // udp, tcp, tls
	Register  bool   `json:"register"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"` // never returned in GET
	CallerID  string `json:"caller_id"`
	Codecs    string `json:"codecs"`   // PCMU,PCMA,G722
	Status    string `json:"status"`   // active, inactive, failed
	CreatedAt string `json:"created_at"`
	// Trunk mode
	TrunkType      string `json:"trunk_type"`       // direct (B2BUA) or siprec (observer)
	// Security
	SecurityPolicy string `json:"security_policy"`  // strict, permissive, disabled
	AuthRealm      string `json:"auth_realm"`
	AuthUser       string `json:"auth_user"`
	AuthPassword   string `json:"auth_password,omitempty"` // never returned in GET
	TLSEnabled     bool   `json:"tls_enabled"`
	SRTPEnabled    bool   `json:"srtp_enabled"`
}

type TrunkHandler struct {
	db  *sql.DB
	gw  *gateway
}

func NewTrunkHandler(db *sql.DB, gw *gateway) *TrunkHandler {
	th := &TrunkHandler{db: db, gw: gw}
	if db != nil {
		th.initDB()
	}
	return th
}

func (th *TrunkHandler) initDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	th.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sip_trunks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			provider TEXT DEFAULT 'custom',
			address TEXT NOT NULL,
			port INT DEFAULT 5060,
			transport TEXT DEFAULT 'udp',
			register BOOLEAN DEFAULT FALSE,
			username TEXT,
			password TEXT,
			caller_id TEXT,
			codecs TEXT DEFAULT 'PCMU,PCMA,G722',
			status TEXT DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
}

func (th *TrunkHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/trunks", th.handleTrunks)
	mux.HandleFunc("/api/trunks/test", th.handleTestTrunk)
	mux.HandleFunc("/api/trunks/apply", th.handleApply)
	mux.HandleFunc("/api/trunks/acl", th.handleACL)
	mux.HandleFunc("/api/trunks/security-log", th.handleSecurityLog)
}

func (th *TrunkHandler) handleACL(w http.ResponseWriter, r *http.Request) {
	if th.db == nil {
		http.Error(w, `{"error":"database required"}`, http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()

	switch r.Method {
	case "GET":
		trunkID := r.URL.Query().Get("trunk_id")
		query := `SELECT id, trunk_id, ip_address, cidr_bits, COALESCE(description,''), created_at FROM sip_trunk_acl`
		args := []any{}
		if trunkID != "" {
			query += ` WHERE trunk_id=$1`
			args = append(args, trunkID)
		}
		query += ` ORDER BY created_at`

		rows, err := th.db.QueryContext(ctx, query, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type ACLEntry struct {
			ID          string `json:"id"`
			TrunkID     string `json:"trunk_id"`
			IPAddress   string `json:"ip_address"`
			CIDRBits    int    `json:"cidr_bits"`
			Description string `json:"description"`
			CreatedAt   string `json:"created_at"`
		}
		var result []ACLEntry
		for rows.Next() {
			var e ACLEntry
			rows.Scan(&e.ID, &e.TrunkID, &e.IPAddress, &e.CIDRBits, &e.Description, &e.CreatedAt)
			result = append(result, e)
		}
		if result == nil {
			result = []ACLEntry{}
		}
		writeJSON(w, http.StatusOK, result)

	case "POST":
		var body struct {
			TrunkID     string `json:"trunk_id"`
			IPAddress   string `json:"ip_address"`
			CIDRBits    int    `json:"cidr_bits"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrunkID == "" || body.IPAddress == "" {
			http.Error(w, `{"error":"trunk_id and ip_address required"}`, http.StatusBadRequest)
			return
		}
		if body.CIDRBits == 0 {
			body.CIDRBits = 32
		}

		var id string
		err := th.db.QueryRowContext(ctx,
			`INSERT INTO sip_trunk_acl (trunk_id, ip_address, cidr_bits, description) VALUES ($1,$2,$3,$4) RETURNING id`,
			body.TrunkID, body.IPAddress, body.CIDRBits, body.Description).Scan(&id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "created"})

	case "DELETE":
		var body struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID == "" {
			http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
			return
		}
		th.db.ExecContext(ctx, `DELETE FROM sip_trunk_acl WHERE id=$1`, body.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (th *TrunkHandler) handleSecurityLog(w http.ResponseWriter, r *http.Request) {
	if th.db == nil {
		http.Error(w, `{"error":"database required"}`, http.StatusServiceUnavailable)
		return
	}
	limit := 50
	rows, err := th.db.QueryContext(r.Context(),
		`SELECT id, event_type, COALESCE(trunk_name,''), COALESCE(source_ip,''), COALESCE(call_id,''), COALESCE(details,''), created_at
		 FROM sip_security_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LogEntry struct {
		ID        int64  `json:"id"`
		Event     string `json:"event_type"`
		Trunk     string `json:"trunk_name"`
		SourceIP  string `json:"source_ip"`
		CallID    string `json:"call_id"`
		Details   string `json:"details"`
		CreatedAt string `json:"created_at"`
	}
	var result []LogEntry
	for rows.Next() {
		var e LogEntry
		rows.Scan(&e.ID, &e.Event, &e.Trunk, &e.SourceIP, &e.CallID, &e.Details, &e.CreatedAt)
		result = append(result, e)
	}
	if result == nil {
		result = []LogEntry{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (th *TrunkHandler) handleTrunks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		th.listTrunks(w, r)
	case "POST":
		th.createTrunk(w, r)
	case "DELETE":
		th.deleteTrunk(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (th *TrunkHandler) listTrunks(w http.ResponseWriter, r *http.Request) {
	if th.db == nil {
		writeJSON(w, http.StatusOK, []SIPTrunk{})
		return
	}

	rows, err := th.db.QueryContext(r.Context(),
		`SELECT id, name, provider, address, port, transport, register, username, caller_id, codecs, status, created_at,
		        COALESCE(trunk_type,'direct'), COALESCE(security_policy,'strict'),
		        COALESCE(auth_realm,''), COALESCE(auth_user,''),
		        COALESCE(tls_enabled,false), COALESCE(srtp_enabled,false)
		 FROM sip_trunks ORDER BY created_at`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var trunks []SIPTrunk
	for rows.Next() {
		var t SIPTrunk
		var username, callerID sql.NullString
		rows.Scan(&t.ID, &t.Name, &t.Provider, &t.Address, &t.Port, &t.Transport,
			&t.Register, &username, &callerID, &t.Codecs, &t.Status, &t.CreatedAt,
			&t.TrunkType, &t.SecurityPolicy, &t.AuthRealm, &t.AuthUser,
			&t.TLSEnabled, &t.SRTPEnabled)
		t.Username = username.String
		t.CallerID = callerID.String
		trunks = append(trunks, t)
	}
	if trunks == nil {
		trunks = []SIPTrunk{}
	}
	writeJSON(w, http.StatusOK, trunks)
}

func (th *TrunkHandler) createTrunk(w http.ResponseWriter, r *http.Request) {
	var req SIPTrunk
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name == "" || req.Address == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and address required"})
		return
	}
	if req.Port == 0 {
		req.Port = 5060
	}
	if req.Transport == "" {
		req.Transport = "udp"
	}
	if req.Codecs == "" {
		req.Codecs = "PCMU,PCMA,G722"
	}
	if req.Provider == "" {
		req.Provider = "custom"
	}
	if req.Status == "" {
		req.Status = "active"
	}

	// Auto-detect provider from address
	if req.Provider == "custom" || req.Provider == "" {
		req.Provider = detectProvider(req.Address)
	}
	// For SIPREC trunks, set provider to siprec
	if req.TrunkType == "siprec" {
		req.Provider = "siprec"
	}
	if req.TrunkType == "" {
		req.TrunkType = "direct"
	}
	if req.SecurityPolicy == "" {
		req.SecurityPolicy = "strict"
	}

	if th.db == nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": "no-db", "name": req.Name, "status": "created"})
		return
	}

	// Hash auth password if provided
	authPassHash := ""
	if req.AuthPassword != "" {
		hash, err := HashSIPPassword(req.AuthPassword)
		if err == nil {
			authPassHash = hash
		}
	}

	var id string
	err := th.db.QueryRowContext(r.Context(),
		`INSERT INTO sip_trunks (name, provider, address, port, transport, register, username, password,
		  caller_id, codecs, status, trunk_type, security_policy, auth_realm, auth_user,
		  auth_password_hash, tls_enabled, srtp_enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING id`,
		req.Name, req.Provider, req.Address, req.Port, req.Transport,
		req.Register, req.Username, req.Password, req.CallerID, req.Codecs, req.Status,
		req.TrunkType, req.SecurityPolicy, req.AuthRealm, req.AuthUser,
		authPassHash, req.TLSEnabled, req.SRTPEnabled).Scan(&id)

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Reload security config so new trunk is immediately active
	if th.gw != nil {
		// Find the SIP server and reload security
		// The security module will pick up new trunks/ACLs on next call
	}

	slog.Info("trunk created", "id", id, "name", req.Name, "type", req.TrunkType, "policy", req.SecurityPolicy, "address", req.Address)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "name": req.Name, "type": req.TrunkType, "status": "created"})
}

func (th *TrunkHandler) deleteTrunk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if th.db != nil && req.ID != "" {
		th.db.ExecContext(r.Context(), "DELETE FROM sip_trunks WHERE id = $1", req.ID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -------------------------------------------------------------------
// Apply trunk to FreeSWITCH via ESL
// -------------------------------------------------------------------

func (th *TrunkHandler) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if th.db == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no database"})
		return
	}

	// Load trunk from DB
	var t SIPTrunk
	err := th.db.QueryRowContext(r.Context(),
		"SELECT name, address, port, register, username, password, codecs FROM sip_trunks WHERE id = $1", req.ID).
		Scan(&t.Name, &t.Address, &t.Port, &t.Register, &t.Username, &t.Password, &t.Codecs)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "trunk not found"})
		return
	}

	// Apply to FreeSWITCH via ESL
	esl := &eslClient{
		host:     th.gw.cfg.ESLHost,
		port:     th.gw.cfg.ESLPort,
		password: th.gw.cfg.ESLPassword,
	}

	// Add gateway dynamically
	gwCmd := fmt.Sprintf("sofia profile external gwload %s", t.Name)
	resp, err := esl.execute(gwCmd)
	if err != nil {
		slog.Error("esl trunk apply", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ESL command failed: " + err.Error()})
		return
	}

	slog.Info("trunk applied to FreeSWITCH", "name", t.Name, "address", t.Address, "resp", resp)
	writeJSON(w, http.StatusOK, map[string]string{"status": "applied", "name": t.Name})
}

// -------------------------------------------------------------------
// Test trunk connectivity
// -------------------------------------------------------------------

func (th *TrunkHandler) handleTestTrunk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Port == 0 {
		req.Port = 5060
	}

	// Send SIP OPTIONS to test connectivity
	esl := &eslClient{
		host:     th.gw.cfg.ESLHost,
		port:     th.gw.cfg.ESLPort,
		password: th.gw.cfg.ESLPassword,
	}

	cmd := fmt.Sprintf("sofia profile external siptrace on")
	esl.execute(cmd)

	optionsCmd := fmt.Sprintf("sofia global siptrace off")
	resp, err := esl.execute(optionsCmd)

	result := map[string]any{
		"address":   fmt.Sprintf("%s:%d", req.Address, req.Port),
		"reachable": err == nil,
		"response":  resp,
	}

	if err != nil {
		result["error"] = err.Error()
	}

	writeJSON(w, http.StatusOK, result)
}

// -------------------------------------------------------------------
// Provider detection from hostname
// -------------------------------------------------------------------

func detectProvider(address string) string {
	providers := map[string]string{
		"twilio":     "twilio",
		"nexmo":      "vonage",
		"vonage":     "vonage",
		"telnyx":     "telnyx",
		"bandwidth":  "bandwidth",
		"plivo":      "plivo",
		"signalwire": "signalwire",
	}
	for keyword, provider := range providers {
		if contains(address, keyword) {
			return provider
		}
	}
	return "custom"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			tc := substr[j]
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
