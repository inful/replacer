package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reportHTML = `<html><body><script>
  window.data = { "trace":[
    {"task_id":"1","tag":"SAMP_A","name":"TRIM:TRIM (SAMP_A)","status":"OK",
     "script":"tool SAMP_A_R1.fastq.gz"}
  ],"summary":[] };
</script></body></html>`

const (
	anonymizeCmd = "anonymize"
)

const timelineHTML = `<html><body><script>
  window.data = { "elapsed":"1s","processes":[{"label":"TRIM:TRIM (SAMP_A)"}] };
</script></body></html>`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return value
}

func TestRunExtract(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "execution_report.html", reportHTML)
	out := filepath.Join(dir, "json")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract", dir, "-o", out}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	payload := readJSON(t, filepath.Join(out, "execution_report.json"))
	trace, _ := payload["trace"].([]any)
	if len(trace) != 1 {
		t.Fatalf("trace entries = %d, want 1", len(trace))
	}
}

func TestRunAnonymize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "execution_report.html", reportHTML)
	writeFile(t, dir, "execution_timeline.html", timelineHTML)
	out := filepath.Join(dir, "anon")

	var stdout, stderr bytes.Buffer
	if code := run([]string{anonymizeCmd, dir, "-o", out}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}

	report := readJSON(t, filepath.Join(out, "execution_report.anonymized.json"))
	trace, _ := report["trace"].([]any)
	entry, _ := trace[0].(map[string]any)
	if entry["tag"] != "sample001" {
		t.Errorf("tag = %v, want sample001", entry["tag"])
	}
	if entry["script"] != "tool sample001_R1.fastq.gz" {
		t.Errorf("script = %v", entry["script"])
	}

	timeline := readJSON(t, filepath.Join(out, "execution_timeline.anonymized.json"))
	processes, _ := timeline["processes"].([]any)
	process, _ := processes[0].(map[string]any)
	if process["label"] != "TRIM:TRIM (sample001)" {
		t.Errorf("label = %v", process["label"])
	}

	mapping := readJSON(t, filepath.Join(out, "sample_map.json"))
	if mapping["SAMP_A"] != "sample001" {
		t.Errorf("mapping = %v", mapping)
	}
}

func TestRunAnonymizeSampleRegexAndExtra(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "execution_report.html", reportHTML)
	out := filepath.Join(dir, "anon")

	var stdout, stderr bytes.Buffer
	args := []string{"anonymize", dir, "-o", out, "--sample-regex", "^SAMP_", "--extra", "OTHER_ID"}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	mapping := readJSON(t, filepath.Join(out, "sample_map.json"))
	if len(mapping) != 2 || mapping["OTHER_ID"] == "" {
		t.Errorf("mapping = %v, want SAMP_A and OTHER_ID", mapping)
	}
}

func TestRunAnonymizeReusesMap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "execution_report.html", reportHTML)
	out := filepath.Join(dir, "anon")
	mapPath := filepath.Join(out, "sample_map.json")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mapPath, []byte(`{"SAMP_A":"sample042"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	args := []string{"anonymize", dir, "-o", out, "--map", mapPath}
	if code := run(args, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	mapping := readJSON(t, mapPath)
	if mapping["SAMP_A"] != "sample042" {
		t.Errorf("mapping = %v, want SAMP_A -> sample042", mapping)
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "replacer ") {
		t.Errorf("stdout = %q, want a version line", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"nope"}, &stdout, &stderr); code == exitOK {
		t.Fatal("exit = 0, want non-zero")
	}
}
