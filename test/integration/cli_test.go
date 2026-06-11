// Package integration verifies the overcf binary end-to-end against a fake
// Cloudflare API, with no real API calls. The binary is built once in
// TestMain, then each test runs it as a subprocess with CLOUDFLARE_API_URL
// pointed at an in-process httptest server.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "overcf-integration")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create temp dir:", err)
		os.Exit(1)
	}

	binPath = filepath.Join(dir, "overcf")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, "../../cmd/overcf")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build overcf: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

// runCLI executes the overcf binary against the fake server with a clean
// environment. Pass a nil fake to run without CLOUDFLARE_API_TOKEN set.
func runCLI(t *testing.T, fake *fakeCF, stdin string, args ...string) cliResult {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}
	if fake != nil {
		cmd.Env = append(cmd.Env,
			"CLOUDFLARE_API_TOKEN="+testToken,
			"CLOUDFLARE_API_URL="+fake.server.URL,
		)
	}

	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("failed to run %s %v: %v", binPath, args, err)
	}

	return cliResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// parseJSON decodes a {"success": true, "data": ...} envelope from stdout.
func parseJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, stdout)
	}
	return out
}

func dataObject(t *testing.T, stdout string) map[string]any {
	t.Helper()

	out := parseJSON(t, stdout)
	if out["success"] != true {
		t.Fatalf("expected success=true, got: %s", stdout)
	}
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got: %s", stdout)
	}
	return data
}

func dataList(t *testing.T, stdout string) []any {
	t.Helper()

	out := parseJSON(t, stdout)
	if out["success"] != true {
		t.Fatalf("expected success=true, got: %s", stdout)
	}
	data, ok := out["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got: %s", stdout)
	}
	return data
}

func TestZoneList(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "zone", "list")
	if res.code != 0 {
		t.Fatalf("exit code %d, stderr: %s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, testZoneName) {
		t.Errorf("table output missing zone name %q:\n%s", testZoneName, res.stdout)
	}

	res = runCLI(t, fake, "", "zone", "list", "--json")
	zones := dataList(t, res.stdout)
	if len(zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(zones))
	}
	zone := zones[0].(map[string]any)
	if zone["id"] != testZoneID || zone["name"] != testZoneName {
		t.Errorf("unexpected zone: %v", zone)
	}
}

func TestZoneGetByName(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "zone", "get", testZoneName, "--json")
	if res.code != 0 {
		t.Fatalf("exit code %d, stderr: %s", res.code, res.stderr)
	}

	zone := dataObject(t, res.stdout)
	if zone["id"] != testZoneID {
		t.Errorf("expected zone ID %s, got %v", testZoneID, zone["id"])
	}
	if !fake.sawRequest("GET /zones/" + testZoneID) {
		t.Error("expected name to be resolved then fetched via GET /zones/{id}")
	}
}

// TestDNSRecordCRUDLifecycle drives a full create -> list -> get -> update ->
// delete cycle through the real binary, asserting both CLI output and the
// requests/state on the fake API.
func TestDNSRecordCRUDLifecycle(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	// Create (zone given by name, exercising zone resolution)
	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--type", "A", "--name", "www", "--content", "192.0.2.1", "--ttl", "300", "--json")
	if res.code != 0 {
		t.Fatalf("create failed with exit code %d, stderr: %s", res.code, res.stderr)
	}

	created := dataObject(t, res.stdout)
	recordID, _ := created["id"].(string)
	if recordID == "" {
		t.Fatalf("create output missing record ID: %s", res.stdout)
	}

	fake.mu.Lock()
	body := fake.lastBody
	fake.mu.Unlock()
	if body["type"] != "A" || body["name"] != "www" || body["content"] != "192.0.2.1" || body["ttl"] != float64(300) {
		t.Errorf("unexpected create request body: %v", body)
	}

	// List
	res = runCLI(t, fake, "", "dns", "list", testZoneName, "--json")
	if res.code != 0 {
		t.Fatalf("list failed with exit code %d, stderr: %s", res.code, res.stderr)
	}
	records := dataList(t, res.stdout)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d: %s", len(records), res.stdout)
	}
	if records[0].(map[string]any)["id"] != recordID {
		t.Errorf("listed record ID mismatch: %v", records[0])
	}

	// Get
	res = runCLI(t, fake, "", "dns", "get", testZoneName, recordID, "--json")
	if res.code != 0 {
		t.Fatalf("get failed with exit code %d, stderr: %s", res.code, res.stderr)
	}
	record := dataObject(t, res.stdout)
	if record["content"] != "192.0.2.1" || record["ttl"] != float64(300) {
		t.Errorf("unexpected record: %v", record)
	}

	// Update content; TTL should be preserved (read-modify-write)
	res = runCLI(t, fake, "", "dns", "update", testZoneName, recordID, "--content", "192.0.2.99")
	if res.code != 0 {
		t.Fatalf("update failed with exit code %d, stderr: %s", res.code, res.stderr)
	}

	res = runCLI(t, fake, "", "dns", "get", testZoneName, recordID, "--json")
	record = dataObject(t, res.stdout)
	if record["content"] != "192.0.2.99" {
		t.Errorf("content not updated: %v", record)
	}
	if record["ttl"] != float64(300) {
		t.Errorf("TTL not preserved across update: %v", record)
	}

	// Delete
	res = runCLI(t, fake, "", "dns", "delete", testZoneName, recordID, "--yes")
	if res.code != 0 {
		t.Fatalf("delete failed with exit code %d, stderr: %s", res.code, res.stderr)
	}
	if fake.recordCount() != 0 {
		t.Errorf("record still present after delete")
	}

	// Get after delete -> NotFound exit code
	res = runCLI(t, fake, "", "dns", "get", testZoneName, recordID)
	if res.code != 3 {
		t.Errorf("expected exit code 3 (NotFound) after delete, got %d", res.code)
	}
}

func TestDNSCreateMXSendsPriority(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--type", "MX", "--name", "@", "--content", "mail.example-test.com", "--priority", "10")
	if res.code != 0 {
		t.Fatalf("exit code %d, stderr: %s", res.code, res.stderr)
	}

	fake.mu.Lock()
	body := fake.lastBody
	fake.mu.Unlock()
	if body["type"] != "MX" || body["priority"] != float64(10) {
		t.Errorf("unexpected MX request body: %v", body)
	}
}

func TestDNSCreateSRVSendsData(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--type", "SRV", "--name", "_sip._tcp",
		"--priority", "10", "--weight", "5", "--port", "5060", "--target", "sip.example-test.com")
	if res.code != 0 {
		t.Fatalf("exit code %d, stderr: %s", res.code, res.stderr)
	}

	fake.mu.Lock()
	body := fake.lastBody
	fake.mu.Unlock()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("SRV request body missing data object: %v", body)
	}
	if data["port"] != float64(5060) || data["weight"] != float64(5) || data["target"] != "sip.example-test.com" {
		t.Errorf("unexpected SRV data: %v", data)
	}
}

func TestDNSCreateFromJSON(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--from-json", `{"type":"TXT","name":"_verify","content":"token=abc123"}`)
	if res.code != 0 {
		t.Fatalf("exit code %d, stderr: %s", res.code, res.stderr)
	}

	fake.mu.Lock()
	body := fake.lastBody
	fake.mu.Unlock()
	if body["type"] != "TXT" || body["content"] != "token=abc123" {
		t.Errorf("unexpected request body: %v", body)
	}
}

func TestDNSCreateInvalidIPFailsValidation(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--type", "A", "--name", "www", "--content", "not-an-ip")
	if res.code != 4 {
		t.Errorf("expected exit code 4 (ValidationError), got %d", res.code)
	}
	if fake.sawRequest("POST /zones/" + testZoneID + "/dns_records") {
		t.Error("invalid record should not reach the API")
	}
}

func TestDNSDeleteWithoutYesIsRefusedNonInteractive(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "create", testZoneName,
		"--type", "A", "--name", "www", "--content", "192.0.2.1", "--json")
	recordID := dataObject(t, res.stdout)["id"].(string)

	res = runCLI(t, fake, "", "dns", "delete", testZoneName, recordID)
	if !strings.Contains(res.stderr, "--yes") {
		t.Errorf("expected stderr to mention --yes, got: %s", res.stderr)
	}
	if fake.recordCount() != 1 {
		t.Error("record was deleted despite missing confirmation")
	}
	if fake.sawRequest("DELETE /zones/" + testZoneID + "/dns_records/" + recordID) {
		t.Error("DELETE request sent despite missing confirmation")
	}
}

func TestZoneNotFoundExitCode(t *testing.T) {
	fake := newFakeCF()
	defer fake.close()

	res := runCLI(t, fake, "", "dns", "list", "no-such-zone.example")
	if res.code != 3 {
		t.Errorf("expected exit code 3 (NotFound), got %d, stderr: %s", res.code, res.stderr)
	}
}

func TestMissingTokenExitCode(t *testing.T) {
	res := runCLI(t, nil, "", "zone", "list")
	if res.code != 2 {
		t.Errorf("expected exit code 2 (AuthError), got %d", res.code)
	}
	if !strings.Contains(res.stderr, "CLOUDFLARE_API_TOKEN") {
		t.Errorf("expected stderr to mention CLOUDFLARE_API_TOKEN, got: %s", res.stderr)
	}
}
