// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Mock Redfish server for firmware testing
func mockRedfishFirmwareServer(t *testing.T, responseDelay time.Duration, maxConcurrent *int32, currentConcurrent *int32) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Track concurrent requests
		if maxConcurrent != nil && currentConcurrent != nil {
			current := atomic.AddInt32(currentConcurrent, 1)
			defer atomic.AddInt32(currentConcurrent, -1)
			// Update max if this is higher
			for {
				max := atomic.LoadInt32(maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt32(maxConcurrent, max, current) {
					break
				}
			}
		}

		// Simulate processing delay
		if responseDelay > 0 {
			time.Sleep(responseDelay)
		}

		// Handle different endpoints
		switch {
		case strings.HasSuffix(r.URL.Path, "/UpdateService/FirmwareInventory/BMC"):
			// Return firmware inventory
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"@odata.id":  r.URL.Path,
				"Id":         "BMC",
				"Name":       "BMC Firmware",
				"Version":    "1.0.0",
				"Updateable": true,
				"Status":     map[string]interface{}{"State": "Enabled"},
			})

		case strings.Contains(r.URL.Path, "/UpdateService/Actions/") || strings.HasSuffix(r.URL.Path, "/UpdateService"):
			if r.Method == "POST" {
				// Handle SimpleUpdate POST
				body, _ := io.ReadAll(r.Body)
				_ = body // we don't need to validate payload here

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"@odata.id": "/redfish/v1/TaskService/Tasks/1",
					"Id":        "1",
					"Name":      "Firmware Update Task",
					"TaskState": "Running",
				})
			} else {
				// GET UpdateService
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"@odata.id": "/redfish/v1/UpdateService",
					"Id":        "UpdateService",
					"Actions": map[string]interface{}{
						"#UpdateService.SimpleUpdate": map[string]interface{}{
							"target": "/redfish/v1/UpdateService/Actions/UpdateService.SimpleUpdate",
						},
					},
				})
			}

		default:
			http.NotFound(w, r)
		}
	})

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server
}

// TestFirmwareParallelExecution tests that parallel execution works correctly
func TestFirmwareParallelExecution(t *testing.T) {
	tests := []struct {
		name              string
		batchSize         int
		numHosts          int
		responseDelay     time.Duration
		expectedParallel  bool
		maxExpectedConcur int32
	}{
		{"serial execution with batch-size 0", 0, 5, 50 * time.Millisecond, false, 1},
		{"serial execution with batch-size 1", 1, 5, 50 * time.Millisecond, false, 1},
		{"parallel execution with batch-size 2", 2, 6, 100 * time.Millisecond, true, 2},
		{"parallel execution with batch-size 5", 5, 10, 100 * time.Millisecond, true, 5},
		{"parallel execution with batch-size greater than hosts", 10, 5, 100 * time.Millisecond, true, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var maxConcurrent, currentConcurrent int32
			server := mockRedfishFirmwareServer(t, tt.responseDelay, &maxConcurrent, &currentConcurrent)

			// Set env
			t.Setenv("REDFISH_USER", "testuser")
			t.Setenv("REDFISH_PASSWORD", "testpass")

			// Create inventory file
			tmpFile, err := os.CreateTemp("", "fw-test-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())

			// Generate BMC entries pointing to mock server
			host := strings.TrimPrefix(server.URL, "https://")
			var bmcs []string
			for i := 0; i < tt.numHosts; i++ {
				bmcs = append(bmcs, fmt.Sprintf("  - xname: x9000c1s%db0\n    ip: %s", i, host))
			}
			inventory := fmt.Sprintf("bmcs:\n%s\n", strings.Join(bmcs, "\n"))
			if _, err := tmpFile.WriteString(inventory); err != nil {
				t.Fatal(err)
			}
			tmpFile.Close()

			// Configure command globals
			fwFile = tmpFile.Name()
			fwType = "bmc"
			fwImageURI = "http://10.0.0.1/firmware.bin"
			fwProtocol = "HTTP"
			fwInsecure = true
			fwTimeout = 5 * time.Second
			fwDryRun = false
			fwBatchSize = tt.batchSize
			fwTargets = nil
			fwExpectedVersion = ""
			fwForce = false

			// Capture combined stdout/stderr
			oldStdout := os.Stdout
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = w
			os.Stderr = w
			defer func() {
				w.Close()
				os.Stdout = oldStdout
				os.Stderr = oldStderr
			}()

			// Run command
			cmd := firmwareCmd
			cmd.SetContext(context.Background())
			start := time.Now()
			err = cmd.RunE(cmd, []string{})
			elapsed := time.Since(start)

			// Read output
			w.Close()
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if err != nil {
				t.Fatalf("unexpected error: %v\nOutput: %s", err, output)
			}

			actualMax := atomic.LoadInt32(&maxConcurrent)
			if actualMax > tt.maxExpectedConcur {
				t.Fatalf("exceeded max concurrent requests: got %d, expected max %d", actualMax, tt.maxExpectedConcur)
			}

			if tt.expectedParallel && actualMax < 2 {
				t.Fatalf("parallel execution expected but max concurrent was %d", actualMax)
			}

			successCount := strings.Count(output, "Triggered firmware update")
			if successCount != tt.numHosts {
				t.Fatalf("expected %d success messages, got %d\nOutput: %s", tt.numHosts, successCount, output)
			}

			// Basic timing heuristic: parallel runs should complete faster than strictly serial
			if tt.expectedParallel {
				// serial time estimate is numHosts * delay; parallel should be notably less
				serialEstimate := time.Duration(tt.numHosts) * tt.responseDelay
				if elapsed > serialEstimate {
					// not fatal, but warn
					t.Logf("note: parallel run elapsed %v, serial estimate %v", elapsed, serialEstimate)
				}
			}

			t.Logf("Max concurrent: %d, Elapsed: %v", actualMax, elapsed)
		})
	}
}

// TestFirmwareDryRunParallel tests dry-run mode with parallelism
func TestFirmwareDryRunParallel(t *testing.T) {
	server := mockRedfishFirmwareServer(t, 0, nil, nil)
	_ = server // server only used to provide a host string if needed

	t.Setenv("REDFISH_USER", "testuser")
	t.Setenv("REDFISH_PASSWORD", "testpass")

	// Create inventory file
	tmpFile, err := os.CreateTemp("", "fw-dryrun-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	inventory := `bmcs:
  - xname: x9000c1s0b0
    ip: 10.1.1.10
  - xname: x9000c1s1b0
    ip: 10.1.1.11
  - xname: x9000c1s2b0
    ip: 10.1.1.12
`
	if _, err := tmpFile.WriteString(inventory); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Configure command
	fwFile = tmpFile.Name()
	fwType = "bmc"
	fwImageURI = "http://10.0.0.1/firmware.bin"
	fwProtocol = "HTTP"
	fwDryRun = true
	fwBatchSize = 3
	fwTargets = nil
	fwExpectedVersion = "1.2.3"
	fwForce = false

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		w.Close()
		os.Stdout = oldStdout
	}()

	cmd := firmwareCmd
	cmd.SetContext(context.Background())
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	dryRunCount := strings.Count(output, "[dry-run]")
	if dryRunCount != 3 {
		t.Fatalf("expected 3 dry-run messages, got %d\nOutput: %s", dryRunCount, output)
	}

	if !strings.Contains(output, "expected-version=1.2.3") {
		t.Fatalf("expected-version not found in dry-run output: %s", output)
	}
}

// TestFirmwareSemaphoreLimiting tests that semaphore correctly limits concurrency
func TestFirmwareSemaphoreLimiting(t *testing.T) {
	var maxConcurrent, currentConcurrent int32
	server := mockRedfishFirmwareServer(t, 200*time.Millisecond, &maxConcurrent, &currentConcurrent)

	t.Setenv("REDFISH_USER", "testuser")
	t.Setenv("REDFISH_PASSWORD", "testpass")

	// Create inventory
	tmpFile, err := os.CreateTemp("", "fw-sem-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	host := strings.TrimPrefix(server.URL, "https://")
	var bmcs []string
	numHosts := 15
	for i := 0; i < numHosts; i++ {
		bmcs = append(bmcs, fmt.Sprintf("  - xname: x9000c1s%db0\n    ip: %s", i, host))
	}
	inventory := fmt.Sprintf("bmcs:\n%s\n", strings.Join(bmcs, "\n"))
	if _, err := tmpFile.WriteString(inventory); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Configure command
	fwFile = tmpFile.Name()
	fwType = "bmc"
	fwImageURI = "http://10.0.0.1/firmware.bin"
	fwProtocol = "HTTP"
	fwInsecure = true
	fwTimeout = 10 * time.Second
	fwDryRun = false
	fwBatchSize = 3
	fwTargets = nil

	// Suppress output
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	os.Stdout, _ = os.Open(os.DevNull)
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	cmd := firmwareCmd
	cmd.SetContext(context.Background())
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	actualMax := atomic.LoadInt32(&maxConcurrent)
	if actualMax > 3 {
		t.Fatalf("semaphore failed to limit concurrency: max was %d, expected 3", actualMax)
	}
	if actualMax < 2 {
		t.Fatalf("parallelism not working: max concurrent was %d, expected >=2", actualMax)
	}

	t.Logf("Max concurrent with batch-size 3: %d", actualMax)
}

// TestDefaultTargets tests the defaultTargets helper function
func TestDefaultTargets(t *testing.T) {
	tests := []struct {
		name        string
		fwType      string
		wantTargets []string
		wantErr     bool
	}{
		{"cc type", "cc", []string{"/redfish/v1/UpdateService/FirmwareInventory/BMC"}, false},
		{"bmc type", "bmc", []string{"/redfish/v1/UpdateService/FirmwareInventory/BMC"}, false},
		{"nc type", "nc", []string{"/redfish/v1/UpdateService/FirmwareInventory/BMC"}, false},
		{"bios type", "bios", []string{"/redfish/v1/UpdateService/FirmwareInventory/Node0.BIOS", "/redfish/v1/UpdateService/FirmwareInventory/Node1.BIOS"}, false},
		{"unknown type", "unknown", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := defaultTargets(tt.fwType)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(targets) != len(tt.wantTargets) {
				t.Fatalf("wrong number of targets: got %d, want %d", len(targets), len(tt.wantTargets))
			}
			for i := range targets {
				if targets[i] != tt.wantTargets[i] {
					t.Fatalf("target[%d] = %q, want %q", i, targets[i], tt.wantTargets[i])
				}
			}
		})
	}
}
