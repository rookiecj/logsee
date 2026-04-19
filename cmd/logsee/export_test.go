package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildBinary compiles the logsee binary into a temp directory. Callers
// use the returned path as an exec target.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "logsee")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return bin
}

func TestExportAnomalies_EndToEnd(t *testing.T) {
	bin := buildBinary(t)
	sample := filepath.Join("..", "..", "testdata", "android", "anr_input_dispatch.log")

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--export-anomalies", sample)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	sawANRFinding := false
	sawANRSpan := false
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var rec exportRecord
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if rec.Type == "finding" && rec.Finding != nil && rec.Finding.Kind.String() == "anr" {
			sawANRFinding = true
		}
		if rec.Type == "span" && rec.Span != nil && rec.Span.Kind.String() == "anr" {
			sawANRSpan = true
		}
	}
	if !sawANRFinding {
		t.Error("expected at least one ANR finding in stdout")
	}
	if !sawANRSpan {
		t.Error("expected at least one ANR span in stdout")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("exported")) {
		t.Errorf("stderr missing summary line: %s", stderr.String())
	}
}

func TestExportAnomalies_StdinInput(t *testing.T) {
	bin := buildBinary(t)

	input := "04-19 14:24:10.456  1245  1301 E ActivityManager: ANR in com.example.app\n" +
		"04-19 14:24:10.457  1245  1301 E ActivityManager: PID: 12345\n"

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--export-anomalies", "-")
	cmd.Stdin = bytes.NewBufferString(input)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("stdout is empty — expected at least one finding for ANR input")
	}
}
