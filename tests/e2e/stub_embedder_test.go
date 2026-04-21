//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newEmbedderStub returns an httptest.Server that responds to POST /embeddings
// with a constant 2048-dimensional non-zero vector for every request.
// All keywords embed to the same vector, guaranteeing vec0 finds every stored chunk
// at cosine similarity 1.0 regardless of which keyword is queried.
func newEmbedderStub(t *testing.T) *httptest.Server {
	t.Helper()

	vec := make([]float32, 2048)
	for i := range vec {
		vec[i] = 0.001
	}

	resp := map[string]any{
		"data": []map[string]any{
			{"embedding": vec},
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal embedder stub response: %v", err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}
