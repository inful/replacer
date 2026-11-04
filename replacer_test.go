package replacer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/inful/replacer"
)

const sampleID = "SAMP_A"

const reportHTML = `<html><body><script>
  window.data = { "trace":[
    {"task_id":"1","tag":"SAMP_A","name":"TRIM:TRIM (SAMP_A)","status":"OK",
     "script":"tool SAMP_A_R1.fastq.gz","workdir":"/w/aa/11"}
  ],"summary":[] };
</script></body></html>`

const timelineHTML = `<html><body><script>
  window.data = { "elapsed":"1s","processes":[{"label":"TRIM:TRIM (SAMP_A)"}] };
</script></body></html>`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestExtractPayload(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "execution_report.html", reportHTML)

	payload, err := replacer.ExtractPayload(path)
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	trace, _ := payload["trace"].([]any)
	if len(trace) != 1 {
		t.Fatalf("trace entries = %d, want 1", len(trace))
	}
	entry, _ := trace[0].(map[string]any)
	if entry["tag"] != sampleID {
		t.Errorf("tag = %v, want SAMP_A", entry["tag"])
	}
}

func TestExtractPayloadKeepsNumbersExact(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "execution_timeline.html", `window.data = {"beginningMillis":1793653593445};`)

	payload, err := replacer.ExtractPayload(path)
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `{"beginningMillis":1793653593445}` {
		t.Errorf("encoded = %s", encoded)
	}
}

func TestExtractPayloadWithoutPayload(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "page.html", "<html><body>no data here</body></html>")

	if _, err := replacer.ExtractPayload(path); err == nil {
		t.Fatal("ExtractPayload: want error, got nil")
	}
}

func TestSamples(t *testing.T) {
	dir := t.TempDir()
	report, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", reportHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	timeline, err := replacer.ExtractPayload(writeFile(t, dir, "execution_timeline.html", timelineHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}

	samples := replacer.Samples([]replacer.Payload{report, timeline})
	if len(samples) != 1 || samples[0] != sampleID {
		t.Errorf("samples = %v, want [SAMP_A]", samples)
	}
}

func TestMappingKeepsExisting(t *testing.T) {
	mapping := replacer.Mapping([]string{sampleID, "SAMP_B"}, map[string]string{sampleID: "sample007"}, "sample")
	if mapping[sampleID] != "sample007" {
		t.Errorf("SAMP_A = %s, want sample007", mapping[sampleID])
	}
	if mapping["SAMP_B"] != "sample008" {
		t.Errorf("SAMP_B = %s, want sample008", mapping["SAMP_B"])
	}
}

func TestAnonymizeReplacesEverywhere(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", reportHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	mapping := replacer.Mapping([]string{sampleID}, nil, "sample")

	anonymized := replacer.Anonymize(payload, mapping)
	trace, _ := anonymized["trace"].([]any)
	entry, _ := trace[0].(map[string]any)
	if entry["tag"] != "sample001" {
		t.Errorf("tag = %v, want sample001", entry["tag"])
	}
	if entry["name"] != "TRIM:TRIM (sample001)" {
		t.Errorf("name = %v", entry["name"])
	}
	if entry["script"] != "tool sample001_R1.fastq.gz" {
		t.Errorf("script = %v", entry["script"])
	}
	if entry["status"] != "OK" {
		t.Errorf("status = %v, want OK", entry["status"])
	}
}

func TestDerivePrefixes(t *testing.T) {
	prefixes := replacer.DerivePrefixes([]string{"TESTA_L002", "TESTA_L001", "SRR123"})
	if len(prefixes) != 1 || prefixes[0] != "TESTA" {
		t.Errorf("prefixes = %v, want [TESTA]", prefixes)
	}
}

func TestAnonymizePrefersLongestId(t *testing.T) {
	mapping := replacer.Mapping([]string{"S1", "S1_L001"}, nil, "sample")
	anonymized := replacer.Anonymize(replacer.Payload{"k": "S1_L001 S1"}, mapping)
	if got := anonymized["k"]; got != "sample002 sample001" {
		t.Errorf("k = %v, want sample002 sample001", got)
	}
}
