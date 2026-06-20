package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

type CustomPIIPattern struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Regex     string `json:"regex"`
	Mask      string `json:"mask"`
	Level     string `json:"level"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
}

type CustomRobocallKeyword struct {
	ID        string  `json:"id"`
	Phrase    string  `json:"phrase"`
	Weight    float64 `json:"weight"`
	Category  string  `json:"category"`
	Enabled   bool    `json:"enabled"`
	IsDefault bool    `json:"is_default"`
}

type BiometricConfigEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SecurityRulesManager struct {
	masker   *PIIMasker
	robocall *RobocallDetector
	bio      *VoiceBiometrics
}

func NewSecurityRulesManager(masker *PIIMasker, robocall *RobocallDetector, bio *VoiceBiometrics) *SecurityRulesManager {
	mgr := &SecurityRulesManager{masker: masker, robocall: robocall, bio: bio}
	mgr.loadCustomRules()
	return mgr
}

func (m *SecurityRulesManager) loadCustomRules() {
	if database == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Load custom PII patterns
	rows, err := database.DB().QueryContext(ctx, `SELECT name, regex, mask, level FROM custom_pii_patterns WHERE enabled=true`)
	if err == nil {
		defer rows.Close()
		count := 0
		for rows.Next() {
			var name, regex, mask, level string
			rows.Scan(&name, &regex, &mask, &level)
			compiled, err := regexp.Compile(regex)
			if err != nil {
				slog.Warn("invalid custom PII regex", "name", name, "err", err)
				continue
			}
			m.masker.patterns = append(m.masker.patterns, piiPattern{
				Name: name, Regex: compiled, Mask: mask, Level: level,
			})
			count++
		}
		if count > 0 {
			slog.Info("custom PII patterns loaded", "count", count)
		}
	}

	// Load custom robocall keywords
	rows2, err := database.DB().QueryContext(ctx, `SELECT phrase, weight FROM custom_robocall_keywords WHERE enabled=true`)
	if err == nil {
		defer rows2.Close()
		count := 0
		for rows2.Next() {
			var phrase string
			var weight float64
			rows2.Scan(&phrase, &weight)
			m.robocall.AddKeyword(phrase, weight)
			count++
		}
		if count > 0 {
			slog.Info("custom robocall keywords loaded", "count", count)
		}
	}

	// Load biometric config
	rows3, err := database.DB().QueryContext(ctx, `SELECT key, value FROM voice_biometric_config`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var key, value string
			rows3.Scan(&key, &value)
			if key == "match_threshold" {
				var t float64
				if json.Unmarshal([]byte(value), &t) == nil && t > 0 {
					m.bio.threshold = t
					slog.Info("voice biometric threshold updated", "threshold", t)
				}
			}
		}
	}
}

func (m *SecurityRulesManager) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/security/rules/pii", m.handlePIIRules)
	mux.HandleFunc("/api/security/rules/robocall", m.handleRobocallRules)
	mux.HandleFunc("/api/security/rules/biometric", m.handleBiometricConfig)
}

// --- PII Rules CRUD ---

func (m *SecurityRulesManager) handlePIIRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		patterns := m.listPIIPatterns()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(patterns)

	case "POST":
		var p CustomPIIPattern
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := regexp.Compile(p.Regex); err != nil {
			http.Error(w, "invalid regex: "+err.Error(), http.StatusBadRequest)
			return
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx,
				`INSERT INTO custom_pii_patterns (name, regex, mask, level) VALUES ($1,$2,$3,$4) ON CONFLICT (name) DO UPDATE SET regex=$2, mask=$3, level=$4`,
				p.Name, p.Regex, p.Mask, p.Level)
		}
		compiled, _ := regexp.Compile(p.Regex)
		m.masker.patterns = append(m.masker.patterns, piiPattern{
			Name: p.Name, Regex: compiled, Mask: p.Mask, Level: p.Level,
		})
		slog.Info("custom PII pattern added", "name", p.Name)
		writeJSON(w, http.StatusOK, map[string]string{"status": "added"})

	case "DELETE":
		var req struct{ Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx, `DELETE FROM custom_pii_patterns WHERE name=$1`, req.Name)
		}
		// Remove from in-memory
		filtered := m.masker.patterns[:0]
		for _, p := range m.masker.patterns {
			if p.Name != req.Name {
				filtered = append(filtered, p)
			}
		}
		m.masker.patterns = filtered
		slog.Info("custom PII pattern removed", "name", req.Name)
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *SecurityRulesManager) listPIIPatterns() []CustomPIIPattern {
	defaultNames := map[string]bool{
		"credit_card": true, "ssn": true, "ssn_spoken": true, "credit_card_spoken": true,
		"cvv": true, "dob": true, "dob_compact": true, "dob_spoken": true, "account_number": true,
	}
	var result []CustomPIIPattern
	for _, p := range m.masker.patterns {
		result = append(result, CustomPIIPattern{
			Name: p.Name, Regex: p.Regex.String(), Mask: p.Mask, Level: p.Level,
			Enabled: true, IsDefault: defaultNames[p.Name],
		})
	}
	return result
}

// --- Robocall Rules CRUD ---

func (m *SecurityRulesManager) handleRobocallRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		keywords := m.listRobocallKeywords()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keywords)

	case "POST":
		var kw CustomRobocallKeyword
		if err := json.NewDecoder(r.Body).Decode(&kw); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if kw.Weight == 0 {
			kw.Weight = 1.0
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx,
				`INSERT INTO custom_robocall_keywords (phrase, weight, category) VALUES ($1,$2,$3) ON CONFLICT (phrase) DO UPDATE SET weight=$2, category=$3`,
				kw.Phrase, kw.Weight, kw.Category)
		}
		m.robocall.AddKeyword(kw.Phrase, kw.Weight)
		slog.Info("custom robocall keyword added", "phrase", kw.Phrase)
		writeJSON(w, http.StatusOK, map[string]string{"status": "added"})

	case "DELETE":
		var req struct{ Phrase string `json:"phrase"` }
		json.NewDecoder(r.Body).Decode(&req)
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx, `DELETE FROM custom_robocall_keywords WHERE phrase=$1`, req.Phrase)
		}
		m.robocall.RemoveKeyword(req.Phrase)
		slog.Info("custom robocall keyword removed", "phrase", req.Phrase)
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *SecurityRulesManager) listRobocallKeywords() []CustomRobocallKeyword {
	return m.robocall.ListKeywords()
}

// --- Biometric Config ---

func (m *SecurityRulesManager) handleBiometricConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		config := []BiometricConfigEntry{
			{Key: "match_threshold", Value: json.Number(fmt.Sprintf("%.2f", m.bio.threshold)).String()},
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			rows, err := database.DB().QueryContext(ctx, `SELECT key, value FROM voice_biometric_config`)
			if err == nil {
				defer rows.Close()
				config = config[:0]
				for rows.Next() {
					var e BiometricConfigEntry
					rows.Scan(&e.Key, &e.Value)
					config = append(config, e)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case "POST":
		var entry BiometricConfigEntry
		if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if database != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			database.DB().ExecContext(ctx,
				`INSERT INTO voice_biometric_config (key, value) VALUES ($1,$2) ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=NOW()`,
				entry.Key, entry.Value)
		}
		if entry.Key == "match_threshold" {
			var t float64
			if json.Unmarshal([]byte(entry.Value), &t) == nil && t > 0 {
				m.bio.threshold = t
			}
		}
		slog.Info("biometric config updated", "key", entry.Key, "value", entry.Value)
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
