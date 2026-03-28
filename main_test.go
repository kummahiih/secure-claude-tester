package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testToken = "test-token"

func postRun(t *testing.T, base string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/run", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func pollResult(t *testing.T, base string, timeout time.Duration) testResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, base+"/results", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var r testResult
		json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if r.Status != "running" && r.Status != "pending" {
			return r
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("timed out waiting for test result")
	return testResult{}
}

func writeScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.sh")
	os.WriteFile(p, []byte("#!/bin/bash\n"+content+"\n"), 0755)
	return p
}

func resetResult() {
	mu.Lock()
	result = testResult{Status: "pending"}
	mu.Unlock()
}

func TestRunSuccess(t *testing.T) {
	resetResult()
	script := writeScript(t, "echo ok")
	t.Setenv("TEST_SCRIPT", script)

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	r := pollResult(t, srv.URL, 10*time.Second)
	if r.Status != "pass" {
		t.Fatalf("expected pass, got %s", r.Status)
	}
	if r.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", r.ExitCode)
	}
}

func TestRunFailure(t *testing.T) {
	resetResult()
	script := writeScript(t, "exit 1")
	t.Setenv("TEST_SCRIPT", script)

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	r := pollResult(t, srv.URL, 10*time.Second)
	if r.Status != "fail" {
		t.Fatalf("expected fail, got %s", r.Status)
	}
	if r.ExitCode != 1 {
		t.Fatalf("expected exit 1, got %d", r.ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	resetResult()
	script := writeScript(t, "sleep 999")
	t.Setenv("TEST_SCRIPT", script)
	t.Setenv("TEST_TIMEOUT", "2")

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	r := pollResult(t, srv.URL, 20*time.Second)
	if r.Status != "fail" {
		t.Fatalf("expected fail, got %s", r.Status)
	}
	if r.ExitCode != 124 {
		t.Fatalf("expected exit 124, got %d", r.ExitCode)
	}
	if r.Output == "" {
		t.Fatal("expected output to mention timeout")
	}
}

func TestRunConflict(t *testing.T) {
	resetResult()
	script := writeScript(t, "sleep 999")
	t.Setenv("TEST_SCRIPT", script)
	// FIX: Reduced from 10 to 2 so the test suite doesn't hang unnecessarily
	t.Setenv("TEST_TIMEOUT", "2")

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	// Wait briefly for the goroutine to start
	time.Sleep(200 * time.Millisecond)

	// Second POST should get 409
	resp2 := postRun(t, srv.URL)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp2.StatusCode)
	}

	// Wait for the first run to finish (timeout) so we don't leak
	pollResult(t, srv.URL, 20*time.Second)
}
