//go:build bdd

package steps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	binaryPath   string
	binaryTmpDir string
	buildOnce    sync.Once
	buildErr     error
)

// minimalPDF is a syntactically valid, minimal PDF-1.4 document with no content.
// It satisfies the ledongthuc/pdf parser's structural requirements.
var minimalPDF = []byte(
	"%PDF-1.4\n" +
		"1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n" +
		"3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n" +
		"xref\n0 4\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000056 00000 n \n" +
		"0000000111 00000 n \n" +
		"trailer\n<</Size 4 /Root 1 0 R>>\n" +
		"startxref\n180\n%%EOF\n",
)

// buildBinary compiles the go-apply binary once per test run.
// It uses runtime.Caller to locate the project root relative to this file.
func buildBinary() (string, error) {
	buildOnce.Do(func() {
		_, callerFile, _, _ := runtime.Caller(0)
		// tests/bdd/steps/helpers.go → project root is 3 dirs up
		root := filepath.Join(filepath.Dir(callerFile), "..", "..", "..")
		tmp, err := os.MkdirTemp("", "go-apply-bdd-*")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(tmp, "go-apply")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/go-apply")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build failed: %w\n%s", err, out)
			return
		}
		binaryPath = bin
		binaryTmpDir = tmp
	})
	return binaryPath, buildErr
}

// CleanupBinary removes the temp directory holding the compiled binary.
// Call from TestMain after the suite finishes.
func CleanupBinary() {
	if binaryTmpDir != "" {
		os.RemoveAll(binaryTmpDir)
	}
}

// runCLI runs the go-apply binary with the given arguments in an isolated environment.
func (s *bddState) runCLI(args ...string) {
	bin, err := buildBinary()
	if err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		return
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+s.tmpHome,
		"XDG_CONFIG_HOME="+filepath.Join(s.tmpHome, ".config"),
		"XDG_DATA_HOME="+filepath.Join(s.tmpHome, ".local", "share"),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	s.lastOutput = stdout.String()
	s.lastError = stderr.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			s.exitCode = exitErr.ExitCode()
		} else {
			s.exitCode = 1
		}
	} else {
		s.exitCode = 0
	}
}

// callMCPTool drives go-apply serve via JSON-RPC stdio.
func (s *bddState) callMCPTool(toolName string, args map[string]any) {
	bin, err := buildBinary()
	if err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		return
	}
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(),
		"HOME="+s.tmpHome,
		"XDG_CONFIG_HOME="+filepath.Join(s.tmpHome, ".config"),
		"XDG_DATA_HOME="+filepath.Join(s.tmpHome, ".local", "share"),
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		return
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		return
	}

	// MCP initialize handshake
	if err := writeJSON(stdin, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "bdd-test", "version": "1.0"},
		},
	}); err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
		}
		cmd.Wait() //nolint:errcheck
		return
	}
	// initialized notification (no id)
	if err := writeJSON(stdin, map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	}); err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
		}
		cmd.Wait() //nolint:errcheck
		return
	}
	// tool call
	if err := writeJSON(stdin, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": toolName, "arguments": args},
	}); err != nil {
		s.lastError = err.Error()
		s.exitCode = 1
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
		}
		cmd.Wait() //nolint:errcheck
		return
	}
	stdin.Close()

	if waitErr := cmd.Wait(); waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			s.exitCode = exitErr.ExitCode()
		} else {
			s.exitCode = 1
			if s.lastError == "" {
				s.lastError = waitErr.Error()
			}
		}
	} else {
		s.exitCode = 0
	}

	output := stdout.String()
	s.lastOutput = extractMCPResult(output)
}

func writeJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}

// extractMCPResult finds the last JSON-RPC response with id=2 in the output
// and extracts the text content from result.content[0].text.
func extractMCPResult(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, `"id"`) {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if string(msg.ID) != "2" {
			continue
		}
		if len(msg.Result.Content) > 0 {
			return msg.Result.Content[0].Text
		}
	}
	return raw
}

// newEmbedderStub creates an httptest server that handles both OpenAI-compatible
// embeddings and chat completions requests. This allows tests to point both the
// embedder client and the orchestrator client at the same stub URL.
func newEmbedderStub() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/embeddings"):
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"data": []map[string]any{
					{"embedding": []float64{0.1, 0.2, 0.3}},
				},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/chat/completions"):
			// Return a minimal valid JD extraction response for keyword extraction.
			jdJSON := `{"title":"Software Engineer","company":"Acme","required":["go"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"choices": []map[string]any{
					{"message": map[string]string{"content": jdJSON}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

// writeConfig writes a config.yaml into the tmpHome directory.
// extra is a map of dot-notation keys to values, e.g. "orchestrator.model" → "my-model".
func (s *bddState) writeConfig(extra map[string]string) {
	cfgDir := filepath.Join(s.tmpHome, ".config", "go-apply")
	os.MkdirAll(cfgDir, 0o700) //nolint:errcheck

	cfg := map[string]any{
		"embedder": map[string]any{
			"base_url": s.stubURL,
			"model":    "nomic-embed-text",
		},
	}
	for k, v := range extra {
		parts := strings.SplitN(k, ".", 2)
		if len(parts) == 2 {
			if _, ok := cfg[parts[0]]; !ok {
				cfg[parts[0]] = map[string]any{}
			}
			cfg[parts[0]].(map[string]any)[parts[1]] = v
		}
	}

	data, _ := yaml.Marshal(cfg)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), data, 0o600) //nolint:errcheck
}
