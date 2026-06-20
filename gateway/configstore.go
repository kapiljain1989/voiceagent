package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"
)

type RuntimeConfig struct {
	LLM     LLMConfig      `json:"llm"`
	Prompts PromptConfig   `json:"prompts"`
	Trunks  []TrunkConfig  `json:"trunks"`
}

type LLMConfig struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Region    string `json:"region"`
	MaxTokens int    `json:"max_tokens"`
}

type PromptConfig struct {
	Interactive string `json:"interactive"`
	CoPilot     string `json:"copilot"`
}

type TrunkConfig struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	Register  bool   `json:"register"`
	CallerID  string `json:"caller_id"`
	Codecs    string `json:"codecs"`
	Status    string `json:"status"`
}

type ConfigStore struct {
	mu       sync.RWMutex
	config   RuntimeConfig
	filePath string
}

func NewConfigStore(filePath string, defaults *Config) *ConfigStore {
	cs := &ConfigStore{
		filePath: filePath,
		config: RuntimeConfig{
			LLM: LLMConfig{
				Provider:  "anthropic-vertex",
				Model:     defaults.ClaudeModel,
				Region:    defaults.GCPRegion,
				MaxTokens: 512,
			},
			Prompts: PromptConfig{
				Interactive: defaults.SystemPrompt,
				CoPilot:     coachSystemPrompt,
			},
			Trunks: []TrunkConfig{},
		},
	}

	if err := cs.loadFromFile(); err != nil {
		slog.Info("no config file found, using defaults", "path", filePath)
	} else {
		slog.Info("config loaded from file", "path", filePath)
	}

	return cs
}

func (cs *ConfigStore) loadFromFile() error {
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return json.Unmarshal(data, &cs.config)
}

func (cs *ConfigStore) saveToFile() error {
	cs.mu.RLock()
	data, err := json.MarshalIndent(cs.config, "", "  ")
	cs.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(cs.filePath, data, 0644)
}

func (cs *ConfigStore) Get() RuntimeConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.config
}

func (cs *ConfigStore) GetLLM() LLMConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.config.LLM
}

func (cs *ConfigStore) GetPrompts() PromptConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.config.Prompts
}

func (cs *ConfigStore) GetTrunks() []TrunkConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	trunks := make([]TrunkConfig, len(cs.config.Trunks))
	copy(trunks, cs.config.Trunks)
	return trunks
}

func (cs *ConfigStore) UpdateLLM(llm LLMConfig) error {
	cs.mu.Lock()
	cs.config.LLM = llm
	cs.mu.Unlock()
	slog.Info("LLM config updated", "model", llm.Model, "region", llm.Region)
	return cs.saveToFile()
}

func (cs *ConfigStore) UpdatePrompts(prompts PromptConfig) error {
	cs.mu.Lock()
	if prompts.Interactive != "" {
		cs.config.Prompts.Interactive = prompts.Interactive
	}
	if prompts.CoPilot != "" {
		cs.config.Prompts.CoPilot = prompts.CoPilot
	}
	cs.mu.Unlock()
	slog.Info("prompts updated")
	return cs.saveToFile()
}

func (cs *ConfigStore) AddTrunk(trunk TrunkConfig) error {
	cs.mu.Lock()
	if trunk.Port == 0 {
		trunk.Port = 5060
	}
	if trunk.Transport == "" {
		trunk.Transport = "udp"
	}
	if trunk.Codecs == "" {
		trunk.Codecs = "PCMU,PCMA,G722"
	}
	if trunk.Status == "" {
		trunk.Status = "active"
	}
	cs.config.Trunks = append(cs.config.Trunks, trunk)
	cs.mu.Unlock()
	slog.Info("trunk added", "name", trunk.Name, "address", trunk.Address)
	return cs.saveToFile()
}

func (cs *ConfigStore) RemoveTrunk(name string) error {
	cs.mu.Lock()
	filtered := cs.config.Trunks[:0]
	for _, t := range cs.config.Trunks {
		if t.Name != name {
			filtered = append(filtered, t)
		}
	}
	cs.config.Trunks = filtered
	cs.mu.Unlock()
	slog.Info("trunk removed", "name", name)
	return cs.saveToFile()
}

// RegisterRoutes adds config management API endpoints
func (cs *ConfigStore) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/settings", cs.handleSettings)
	mux.HandleFunc("/api/settings/llm", cs.handleLLM)
	mux.HandleFunc("/api/settings/prompts", cs.handlePrompts)
	mux.HandleFunc("/api/settings/trunks", cs.handleTrunks)
}

func (cs *ConfigStore) handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cs.Get())
}

func (cs *ConfigStore) handleLLM(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cs.GetLLM())
	case "POST", "PUT":
		var llm LLMConfig
		if err := json.NewDecoder(r.Body).Decode(&llm); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cs.UpdateLLM(llm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (cs *ConfigStore) handlePrompts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cs.GetPrompts())
	case "POST", "PUT":
		var prompts PromptConfig
		if err := json.NewDecoder(r.Body).Decode(&prompts); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cs.UpdatePrompts(prompts); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (cs *ConfigStore) handleTrunks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cs.GetTrunks())
	case "POST":
		var trunk TrunkConfig
		if err := json.NewDecoder(r.Body).Decode(&trunk); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cs.AddTrunk(trunk); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	case "DELETE":
		var req struct{ Name string `json:"name"` }
		json.NewDecoder(r.Body).Decode(&req)
		if err := cs.RemoveTrunk(req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
