package replacer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/inful/replacer"
)

const (
	sampleID = "SAMP_A"
	laneName = "TESTA"
)

const reportHTML = `<html><body><script>
  window.data = { "trace":[
    {"task_id":"1","tag":"SAMP_A","name":"TRIM:TRIM (SAMP_A)","status":"OK",
     "script":"tool SAMP_A_R1.fastq.gz","workdir":"/w/aa/11"}
  ],"summary":[] };
</script></body></html>`

const timelineHTML = `<html><body><script>
  window.data = { "elapsed":"1s","processes":[{"label":"TRIM:TRIM (SAMP_A)"}] };
</script></body></html>`

// laneHTML is a report whose sample sits on two lanes: the lane and the
// read markers are not part of the sample name.
const laneHTML = `<html><body><script>
  window.data = { "trace":[
    {"task_id":"1","tag":"TESTA_L001","name":"TRIM:TRIM (TESTA_L001)","status":"OK",
     "script":"tool TESTA_S1_L001_R1_001.fastq.gz"},
    {"task_id":"2","tag":"TESTA_L002","name":"TRIM:TRIM (TESTA_L002)","status":"OK"}
  ],"summary":[] };
</script></body></html>`

// accessionHTML mimics a report whose sample names are accessions: one
// letter and seven digits, or eight digits, sometimes with a lane. The
// summary labels the samples the way the report does.
const accessionHTML = `<html><body><script>
  window.data = { "trace":[
    {"task_id":"1","tag":"N1234567","name":"VARSCALL:VARSCALL (N1234567)","status":"OK",
     "script":"sentieon N1234567_R1_001.vcf"},
    {"task_id":"2","tag":"12345678_L001","name":"VARSCALL:VARSCALL (12345678_L001)","status":"OK"},
    {"task_id":"3","tag":"12345678_L002","name":"VARSCALL:VARSCALL (12345678_L002)","status":"OK"}
  ],"summary":[
    {"process":"VARSCALL","time":{"min":1,"minLabel":"VARSCALL:VARSCALL (N1234567)",
      "maxLabel":"VARSCALL:VARSCALL (12345678_L002)"}}
  ] };
</script></body></html>`

// summaryOnlyHTML is a report with no traced task: the sample name survives
// only in the per-process summary labels.
const summaryOnlyHTML = `<html><body><script>
  window.data = { "trace":[],
    "summary":[
      {"process":"VARSCALL","time":{"minLabel":"VARSCALL:VARSCALL (N1234567)"}}
    ] };
</script></body></html>`

func TestSamplesReadsTheSummaryLabels(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", summaryOnlyHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}

	samples := replacer.Samples([]replacer.Payload{payload})
	if len(samples) != 1 || samples[0] != "N1234567" {
		t.Errorf("samples = %v, want [N1234567]", samples)
	}
}

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

func TestSamplesReadsAccessionNames(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", accessionHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}

	samples := replacer.Samples([]replacer.Payload{payload})
	if len(samples) != 2 || samples[0] != "12345678" || samples[1] != "N1234567" {
		t.Errorf("samples = %v, want [12345678 N1234567]", samples)
	}
}

func TestAnonymizeRenamesAccessionsEverywhere(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", accessionHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	mapping := replacer.Mapping(replacer.Samples([]replacer.Payload{payload}), nil, "sample")

	anonymized := replacer.Anonymize(payload, mapping)
	trace, _ := anonymized["trace"].([]any)
	first, _ := trace[0].(map[string]any)
	second, _ := trace[1].(map[string]any)
	if first["tag"] != "sample002" {
		t.Errorf("tag = %v, want sample002", first["tag"])
	}
	if first["script"] != "sentieon sample002_R1_001.vcf" {
		t.Errorf("script = %v", first["script"])
	}
	if second["tag"] != "sample001_L001" {
		t.Errorf("tag = %v, want sample001_L001", second["tag"])
	}
	if second["name"] != "VARSCALL:VARSCALL (sample001_L001)" {
		t.Errorf("name = %v", second["name"])
	}

	// The summary labels the tasks too, and they carry the sample name.
	summary, _ := anonymized["summary"].([]any)
	entry, _ := summary[0].(map[string]any)
	time, _ := entry["time"].(map[string]any)
	if time["minLabel"] != "VARSCALL:VARSCALL (sample002)" {
		t.Errorf("minLabel = %v", time["minLabel"])
	}
	if time["maxLabel"] != "VARSCALL:VARSCALL (sample001_L002)" {
		t.Errorf("maxLabel = %v", time["maxLabel"])
	}
}

func TestSampleName(t *testing.T) {
	cases := map[string]string{
		"TESTA_L001":           laneName,
		"TESTA_S1_L001_R2_001": laneName,
		"TESTA":                "TESTA",
		"SAMP_A":               "SAMP_A",
		"NA12878":              "NA12878",
		"PATIENT_007":          "PATIENT_007",
		// An id that starts with a marker has no name to extract.
		"L001_only": "L001_only",
	}
	for id, want := range cases {
		if got := replacer.SampleName(id); got != want {
			t.Errorf("SampleName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestSamplesGivesOneNamePerSample(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", laneHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}

	samples := replacer.Samples([]replacer.Payload{payload})
	if len(samples) != 1 || samples[0] != laneName {
		t.Errorf("samples = %v, want [%s]", samples, laneName)
	}
}

func TestAnonymizeKeepsTheLane(t *testing.T) {
	dir := t.TempDir()
	payload, err := replacer.ExtractPayload(writeFile(t, dir, "execution_report.html", laneHTML))
	if err != nil {
		t.Fatalf("ExtractPayload: %v", err)
	}
	mapping := replacer.Mapping(replacer.Samples([]replacer.Payload{payload}), nil, "sample")

	anonymized := replacer.Anonymize(payload, mapping)
	trace, _ := anonymized["trace"].([]any)
	entry, _ := trace[0].(map[string]any)
	if entry["tag"] != "sample001_L001" {
		t.Errorf("tag = %v, want sample001_L001", entry["tag"])
	}
	if entry["name"] != "TRIM:TRIM (sample001_L001)" {
		t.Errorf("name = %v", entry["name"])
	}
	if entry["script"] != "tool sample001_S1_L001_R1_001.fastq.gz" {
		t.Errorf("script = %v", entry["script"])
	}
}

func TestAnonymizePrefersLongestId(t *testing.T) {
	mapping := replacer.Mapping([]string{"S1", "S1_L001"}, nil, "sample")
	anonymized := replacer.Anonymize(replacer.Payload{"k": "S1_L001 S1"}, mapping)
	if got := anonymized["k"]; got != "sample002 sample001" {
		t.Errorf("k = %v, want sample002 sample001", got)
	}
}
