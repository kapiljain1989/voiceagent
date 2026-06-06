package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// -------------------------------------------------------------------
// RAG pipeline — ChromaDB vector store for document retrieval
// -------------------------------------------------------------------

type RAGClient struct {
	chromaURL  string
	collection string
}

type RAGChunk struct {
	Text     string  `json:"text"`
	DocName  string  `json:"doc_name"`
	Score    float64 `json:"score"`
	ChunkIdx int     `json:"chunk_index"`
}

func NewRAGClient(chromaURL string) *RAGClient {
	return &RAGClient{
		chromaURL:  chromaURL,
		collection: "voiceagent_docs",
	}
}

// EnsureCollection creates the ChromaDB collection if it doesn't exist.
func (r *RAGClient) EnsureCollection(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{
		"name": r.collection,
		"metadata": map[string]string{
			"description": "VoiceAgent knowledge base documents",
		},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		r.chromaURL+"/api/v2/tenants/default_tenant/databases/default_database/collections",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return nil
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create collection %d: %s", resp.StatusCode, b)
	}
	slog.Info("chromadb collection created", "name", r.collection)
	return nil
}

// IndexDocument chunks text and stores it in ChromaDB.
func (r *RAGClient) IndexDocument(ctx context.Context, docID, docName, text string, chunkSize int) (int, error) {
	chunks := chunkText(text, chunkSize)
	if len(chunks) == 0 {
		return 0, nil
	}

	collectionID, err := r.getCollectionID(ctx)
	if err != nil {
		return 0, err
	}

	ids := make([]string, len(chunks))
	documents := make([]string, len(chunks))
	metadatas := make([]map[string]any, len(chunks))

	embeddings := make([][]float32, len(chunks))
	for i, chunk := range chunks {
		ids[i] = fmt.Sprintf("%s_chunk_%d", docID, i)
		documents[i] = chunk
		metadatas[i] = map[string]any{
			"doc_id":      docID,
			"doc_name":    docName,
			"chunk_index": i,
		}
		embeddings[i] = simpleEmbed(chunk)
	}

	body, _ := json.Marshal(map[string]any{
		"ids":        ids,
		"documents":  documents,
		"metadatas":  metadatas,
		"embeddings": embeddings,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections/%s/add", r.chromaURL, collectionID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("add documents %d: %s", resp.StatusCode, b)
	}

	slog.Info("indexed document", "doc", docName, "chunks", len(chunks))
	return len(chunks), nil
}

// Query retrieves the top-K most relevant chunks for a query string.
func (r *RAGClient) Query(ctx context.Context, queryText string, topK int) ([]RAGChunk, error) {
	collectionID, err := r.getCollectionID(ctx)
	if err != nil {
		return nil, err
	}

	queryEmb := simpleEmbed(queryText)
	body, _ := json.Marshal(map[string]any{
		"query_embeddings": [][]float32{queryEmb},
		"n_results":        topK,
		"include":          []string{"documents", "metadatas", "distances"},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections/%s/query", r.chromaURL, collectionID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Documents [][]string           `json:"documents"`
		Metadatas [][]map[string]any   `json:"metadatas"`
		Distances [][]float64          `json:"distances"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	var chunks []RAGChunk
	if len(result.Documents) > 0 {
		for i, doc := range result.Documents[0] {
			chunk := RAGChunk{Text: doc}
			if i < len(result.Distances[0]) {
				chunk.Score = 1.0 - result.Distances[0][i]
			}
			if i < len(result.Metadatas[0]) {
				if name, ok := result.Metadatas[0][i]["doc_name"].(string); ok {
					chunk.DocName = name
				}
				if idx, ok := result.Metadatas[0][i]["chunk_index"].(float64); ok {
					chunk.ChunkIdx = int(idx)
				}
			}
			chunks = append(chunks, chunk)
		}
	}

	return chunks, nil
}

// DeleteDocument removes all chunks for a document from ChromaDB.
func (r *RAGClient) DeleteDocument(ctx context.Context, docID string) error {
	collectionID, err := r.getCollectionID(ctx)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]any{
		"where": map[string]string{"doc_id": docID},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections/%s/delete", r.chromaURL, collectionID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// BuildRAGContext queries ChromaDB and formats the results as context for the LLM.
func (r *RAGClient) BuildRAGContext(ctx context.Context, query string, topK int) string {
	chunks, err := r.Query(ctx, query, topK)
	if err != nil || len(chunks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Relevant knowledge base context:\n")
	for i, c := range chunks {
		fmt.Fprintf(&b, "[%d] (%s, %.0f%% match): %s\n", i+1, c.DocName, c.Score*100, c.Text)
	}
	return b.String()
}

func (r *RAGClient) getCollectionID(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections/%s", r.chromaURL, r.collection),
		nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ID == "" {
		return r.collection, nil
	}
	return result.ID, nil
}

// simpleEmbed generates a basic 384-dim embedding using character n-gram hashing.
// Suitable for demo/MVP. Production should use Vertex AI text-embedding-004.
func simpleEmbed(text string) []float32 {
	const dim = 384
	emb := make([]float32, dim)
	text = strings.ToLower(text)
	words := strings.Fields(text)
	for _, word := range words {
		for i := 0; i < len(word); i++ {
			for j := i + 1; j <= len(word) && j <= i+4; j++ {
				ngram := word[i:j]
				h := uint32(0)
				for _, c := range ngram {
					h = h*31 + uint32(c)
				}
				idx := h % uint32(dim)
				emb[idx] += 1.0
			}
		}
	}
	// L2 normalize
	var norm float32
	for _, v := range emb {
		norm += v * v
	}
	if norm > 0 {
		norm = float32(1.0 / float64(norm))
		for i := range emb {
			emb[i] *= norm
		}
	}
	return emb
}

// chunkText splits text into chunks of approximately chunkSize characters
// with a 50-character overlap for context continuity.
func chunkText(text string, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	overlap := 50
	if overlap >= chunkSize {
		overlap = 0
	}

	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}

	var chunks []string
	for start := 0; start < len(text); {
		end := start + chunkSize
		if end >= len(text) {
			chunks = append(chunks, strings.TrimSpace(text[start:]))
			break
		}
		// Try to break at a sentence boundary
		breakAt := end
		for i := end; i > start+chunkSize/2; i-- {
			if text[i] == '.' || text[i] == '\n' {
				breakAt = i + 1
				break
			}
		}
		chunks = append(chunks, strings.TrimSpace(text[start:breakAt]))
		start = breakAt - overlap
		if start < 0 {
			start = 0
		}
	}
	return chunks
}
