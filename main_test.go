package main

import (
	"crypto/tls"
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

func TestTLSMinVersion13(t *testing.T) {
	ts := httptest.NewUnstartedServer(setupRouter(testToken))
	ts.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	ts.StartTLS()
	defer ts.Close()

	// TLS 1.3 client — must succeed.
	resp, err := ts.Client().Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("TLS 1.3 client failed: %v", err)
	}
	resp.Body.Close()

	// TLS 1.2 max client — must be rejected.
	base := ts.Client().Transport.(*http.Transport).Clone()
	base.TLSClientConfig.MaxVersion = tls.VersionTLS12
	_, err = (&http.Client{Transport: base}).Get(ts.URL + "/health")
	if err == nil {
		t.Error("expected TLS 1.2 client to be rejected")
	}
}

func getResultsWait(t *testing.T, base string, wait string) testResult {
	t.Helper()
	url := base + "/results"
	if wait != "" {
		url += "?wait=" + wait
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r testResult
	json.NewDecoder(resp.Body).Decode(&r)
	return r
}

func TestResultsWaitBlocks(t *testing.T) {
	resetResult()
	script := writeScript(t, "echo ok")
	t.Setenv("TEST_SCRIPT", script)

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	// wait=true should block until the fast script completes
	r := getResultsWait(t, srv.URL, "true")
	if r.Status != "pass" {
		t.Fatalf("expected pass, got %s", r.Status)
	}
}

func TestResultsWaitFalseImmediate(t *testing.T) {
	resetResult()
	script := writeScript(t, "sleep 999")
	t.Setenv("TEST_SCRIPT", script)
	t.Setenv("TEST_TIMEOUT", "3")

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	// give goroutine a moment to start
	time.Sleep(100 * time.Millisecond)

	// wait=false (or absent) should return immediately with running status
	r := getResultsWait(t, srv.URL, "false")
	if r.Status != "running" {
		t.Fatalf("expected running, got %s", r.Status)
	}

	// clean up: wait for the short timeout to expire
	pollResult(t, srv.URL, 15*time.Second)
}

func TestResultsWaitTimeout(t *testing.T) {
	resetResult()
	script := writeScript(t, "sleep 999")
	t.Setenv("TEST_SCRIPT", script)
	t.Setenv("TEST_TIMEOUT", "2")

	srv := httptest.NewServer(setupRouter(testToken))
	defer srv.Close()

	resp := postRun(t, srv.URL)
	resp.Body.Close()

	// wait=true with a short TEST_TIMEOUT should eventually return (not hang)
	done := make(chan testResult, 1)
	go func() {
		done <- getResultsWait(t, srv.URL, "true")
	}()

	select {
	case r := <-done:
		// should be fail due to timeout
		if r.Status != "fail" {
			t.Fatalf("expected fail after timeout, got %s", r.Status)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("wait=true did not return within expected time")
	}
}
