package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/oauth2/google"
)

// -------------------------------------------------------------------
// LLM abstraction — supports Claude and Gemini on Vertex AI
// -------------------------------------------------------------------

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMClient interface {
	Chat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (string, error)
	StreamChat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (<-chan string, error)
	Name() string
}

type LLMProvider struct {
	creds     *google.Credentials
	projectID string
	region    string
}

func NewLLMProvider(creds *google.Credentials, projectID, region string) *LLMProvider {
	return &LLMProvider{creds: creds, projectID: projectID, region: region}
}

func (p *LLMProvider) Client(provider, model string) LLMClient {
	switch provider {
	case "gemini-vertex":
		return &GeminiClient{provider: p, model: model}
	default:
		return &ClaudeClient{provider: p, model: model}
	}
}

func (p *LLMProvider) token(ctx context.Context) (string, error) {
	tok, err := p.creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("gcp token: %w", err)
	}
	return tok.AccessToken, nil
}

// -------------------------------------------------------------------
// Claude on Vertex AI
// -------------------------------------------------------------------

type ClaudeClient struct {
	provider *LLMProvider
	model    string
}

func (c *ClaudeClient) Name() string { return "claude:" + c.model }

func (c *ClaudeClient) Chat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:rawPredict",
		c.provider.region, c.provider.projectID, c.provider.region, c.model,
	)

	msgs := make([]claudeMessage, len(messages))
	for i, m := range messages {
		msgs[i] = claudeMessage{Role: m.Role, Content: m.Content}
	}

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        maxTokens,
		"system":            systemPrompt,
		"messages":          msgs,
	})

	tok, err := c.provider.token(ctx)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("claude %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Content[0].Text, nil
}

func (c *ClaudeClient) StreamChat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (<-chan string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:streamRawPredict",
		c.provider.region, c.provider.projectID, c.provider.region, c.model,
	)

	msgs := make([]claudeMessage, len(messages))
	for i, m := range messages {
		msgs[i] = claudeMessage{Role: m.Role, Content: m.Content}
	}

	body, _ := json.Marshal(map[string]any{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        maxTokens,
		"system":            systemPrompt,
		"messages":          msgs,
		"stream":            true,
	})

	tok, err := c.provider.token(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("claude %d: %s", resp.StatusCode, b)
	}

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var evt struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &evt) != nil {
				continue
			}
			if evt.Type == "message_stop" {
				return
			}
			if evt.Type == "content_block_delta" && evt.Delta.Type == "text_delta" {
				ch <- evt.Delta.Text
			}
		}
	}()

	return ch, nil
}

// -------------------------------------------------------------------
// Gemini on Vertex AI
// -------------------------------------------------------------------

type GeminiClient struct {
	provider *LLMProvider
	model    string
}

func (g *GeminiClient) Name() string { return "gemini:" + g.model }

func (g *GeminiClient) Chat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		g.provider.region, g.provider.projectID, g.provider.region, g.model,
	)

	contents := make([]map[string]any, len(messages))
	for i, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents[i] = map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": m.Content}},
		}
	}

	reqBody := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": maxTokens,
			"temperature":     0.7,
		},
	}
	if systemPrompt != "" {
		reqBody["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		}
	}

	body, _ := json.Marshal(reqBody)
	tok, err := g.provider.token(ctx)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (g *GeminiClient) StreamChat(ctx context.Context, messages []LLMMessage, systemPrompt string, maxTokens int) (<-chan string, error) {
	url := fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
		g.provider.region, g.provider.projectID, g.provider.region, g.model,
	)

	contents := make([]map[string]any, len(messages))
	for i, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents[i] = map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": m.Content}},
		}
	}

	reqBody := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": maxTokens,
			"temperature":     0.7,
		},
	}
	if systemPrompt != "" {
		reqBody["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		}
	}

	body, _ := json.Marshal(reqBody)
	tok, err := g.provider.token(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, b)
	}

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var evt struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if json.Unmarshal([]byte(data), &evt) != nil {
				continue
			}
			if len(evt.Candidates) > 0 && len(evt.Candidates[0].Content.Parts) > 0 {
				ch <- evt.Candidates[0].Content.Parts[0].Text
			}
		}
	}()

	return ch, nil
}
