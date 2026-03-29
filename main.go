package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// testResult holds the outcome of the most recent test run.
type testResult struct {
	Status    string `json:"status"` // "pass", "fail", "running", or "pending"
	ExitCode  int    `json:"exit_code"`
	Timestamp string `json:"timestamp"`
	Output    string `json:"output"`
}

var (
	mu     sync.Mutex
	result = testResult{Status: "pending"}
)

const defaultTestScript = "/workspace/test.sh"

// handleRun executes test.sh from the mounted workspace and records the result.
// POST /run — returns {"status":"started"} immediately; run is async.
func handleRun(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !verifyToken(r, token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Reject if a run is already in progress.
		mu.Lock()
		if result.Status == "running" {
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "test run already in progress"})
			return
		}
		result = testResult{Status: "running", Timestamp: time.Now().UTC().Format(time.RFC3339)}
		mu.Unlock()

		testScript := os.Getenv("TEST_SCRIPT")
		if testScript == "" {
			testScript = defaultTestScript
		}

		// Check that the script exists before spawning.
		if _, err := os.Stat(testScript); os.IsNotExist(err) {
			mu.Lock()
			result = testResult{
				Status:    "fail",
				ExitCode:  1,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Output:    "test script not found: " + testScript,
			}
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "failed", "error": "test script not found"})
			return
		}

		// Respond immediately; run test asynchronously.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})

		go func() {
			timeoutSec := 300
			if v := os.Getenv("TEST_TIMEOUT"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					timeoutSec = n
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", testScript)
			cmd.Dir = filepath.Dir(testScript)
			cmd.WaitDelay = 10 * time.Second
			cmd.Env = append(os.Environ(),
				"HOME=/home/appuser",
			)

			out, err := cmd.CombinedOutput()

			exitCode := 0
			status := "pass"
			if ctx.Err() == context.DeadlineExceeded {
				exitCode = 124
				status = "fail"
				if len(out) == 0 {
					out = []byte("test timed out after " + strconv.Itoa(timeoutSec) + "s")
				} else {
					out = append(out, []byte("\ntest timed out after "+strconv.Itoa(timeoutSec)+"s")...)
				}
			} else if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
					if len(out) == 0 {
						out = []byte("execution failed to start: " + err.Error())
					}
				}
				status = "fail"
			}

			mu.Lock()
			result = testResult{
				Status:    status,
				ExitCode:  exitCode,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Output:    string(out),
			}
			mu.Unlock()

			log.Printf("RUN_COMPLETE: status=%s exit_code=%d", status, exitCode)
		}()
	}
}

// handleResults returns the stored test result as JSON.
// GET /results
func handleResults(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !verifyToken(r, token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		mu.Lock()
		res := result
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
	}
}

// handleHealth returns 200 OK with no authentication required.
func handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// setupRouter isolates the routing logic so it can be tested independently.
func setupRouter(token string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/run", handleRun(token))
	mux.HandleFunc("/results", handleResults(token))
	mux.HandleFunc("/health", handleHealth())
	return mux
}

// verifyToken checks the Bearer token in the Authorization header.
func verifyToken(r *http.Request, expectedToken string) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	expectedBytes := []byte(expectedToken)
	providedBytes := []byte(parts[1])

	// ConstantTimeCompare requires equal lengths.
	if len(expectedBytes) != len(providedBytes) {
		return false
	}

	return subtle.ConstantTimeCompare(providedBytes, expectedBytes) == 1
}

func main() {
	token := os.Getenv("TESTER_API_TOKEN")
	if token == "" {
		log.Fatal("TESTER_API_TOKEN is required")
	}

	mux := setupRouter(token)

	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	}

	log.Println("Tester server listening on :8443 with TLS")
	log.Fatal(server.ListenAndServeTLS("/app/certs/tester.crt", "/app/certs/tester.key"))
}
