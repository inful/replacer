// Package replacer extracts the machine-readable JSON that Nextflow embeds in
// its execution report and timeline, and renames the sample ids inside it.
//
// Nextflow renders both files from a template that assigns the data to a
// global:
//
//	window.data = { "trace": [...], "summary": [...] };   // execution_report.html
//	window.data = { "elapsed": "...", "processes": [...] }; // execution_timeline.html
//
// The report carries the per-task trace (tag, name, script, workdir), the
// timeline carries the per-process timeline. Sample ids appear in the tag,
// in the "PROCESS:ALIAS (TAG)" label and inside the task script, so they are
// renamed everywhere at once.
package replacer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// File mode for the files the tool writes.
const fileMode = 0o600

// Payload is the decoded JSON object embedded in a Nextflow report or timeline.
type Payload = map[string]any

// payloadMarkers are the assignments the Nextflow templates use to hand the
// data to the page scripts. The first is the current one; the others cover
// older templates that assigned a plain variable.
var payloadMarkers = []string{
	"window.data =",
	"var data =",
	"const data =",
	"let data =",
}

// tagLabel matches Nextflow's "PROCESS:ALIAS (TAG)" task label.
var tagLabel = regexp.MustCompile(`^(.*)\(([^()]+)\)\s*$`)

// ExtractPayload reads path and returns the JSON payload embedded in it.
func ExtractPayload(path string) (Payload, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the user's own input
	if err != nil {
		return nil, err
	}
	return payloadFromHTML(raw, path)
}

// payloadFromHTML returns the JSON payload assigned by the page scripts.
func payloadFromHTML(raw []byte, path string) (Payload, error) {
	for _, marker := range payloadMarkers {
		for offset := 0; offset < len(raw); {
			idx := bytes.Index(raw[offset:], []byte(marker))
			if idx < 0 {
				break
			}
			idx += offset
			payload, err := decodePayload(raw[idx+len(marker):])
			if err == nil {
				return payload, nil
			}
			offset = idx + len(marker)
		}
	}
	return nil, fmt.Errorf("%s: no Nextflow JSON payload found", path)
}

// decodePayload decodes the first JSON value in raw. Numbers are kept as
// json.Number so a round trip through the tool does not reformat them.
func decodePayload(raw []byte) (Payload, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var payload Payload
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("empty payload")
	}
	return payload, nil
}

// LoadPayload reads a Nextflow report/timeline HTML file or a JSON file
// holding a payload.
func LoadPayload(path string) (Payload, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the user's own input
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(path), ".html") {
		return payloadFromHTML(raw, path)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var payload Payload
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return payload, nil
}

// Samples returns the sample ids detected in payloads: the tag of every
// traced task plus the id in a "PROCESS:ALIAS (TAG)" label. The result is
// sorted and free of duplicates.
func Samples(payloads []Payload) []string {
	found := map[string]bool{}
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if !nameLike(candidate) {
			return
		}
		found[candidate] = true
	}
	for _, payload := range payloads {
		trace, _ := payload["trace"].([]any)
		for _, entry := range trace {
			task, _ := entry.(map[string]any)
			if tag, ok := task["tag"].(string); ok {
				add(tag)
			}
		}
		walk(payload, "", func(key string, value any) {
			if key != "name" && key != "label" {
				return
			}
			label, ok := value.(string)
			if !ok {
				return
			}
			if match := tagLabel.FindStringSubmatch(strings.TrimSpace(label)); match != nil {
				add(match[2])
			}
		})
	}
	samples := make([]string, 0, len(found))
	for sample := range found {
		samples = append(samples, sample)
	}
	sort.Strings(samples)
	return samples
}

// nameLike reports whether s looks like a name rather than a bare number.
func nameLike(s string) bool {
	if s == "" || s == "-" {
		return false
	}
	digits := true
	for _, r := range s {
		if !unicode.IsDigit(r) {
			digits = false
			break
		}
	}
	return !digits
}

// walk calls fn for every value in the payload, with the key it was stored
// under. Keys inside a list are the key of the list itself.
func walk(node any, key string, fn func(key string, value any)) {
	fn(key, node)
	switch typed := node.(type) {
	case map[string]any:
		for k, v := range typed {
			walk(v, k, fn)
		}
	case []any:
		for _, v := range typed {
			walk(v, key, fn)
		}
	}
}

// DerivePrefixes returns the part before the first underscore of every id.
// A tag such as "TESTA_L002" carries the bare sample name "TESTA", which is
// what file names in the task script are built from.
func DerivePrefixes(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	prefixes := make([]string, 0, len(ids))
	for _, id := range ids {
		prefix, _, ok := strings.Cut(id, "_")
		if !ok || prefix == "" || prefix == id || seen[prefix] {
			continue
		}
		seen[prefix] = true
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return prefixes
}

// Mapping maps every sample id to a pseudonym such as "sample001". Ids
// already present in existing keep their pseudonym, so the same mapping can
// be reused across reports. New pseudonyms continue after the highest one
// already in use and never collide with it.
func Mapping(samples []string, existing map[string]string, prefix string) map[string]string {
	mapping := make(map[string]string, len(existing)+len(samples))
	used := make(map[string]bool, len(existing))
	next := 1
	for id, pseudo := range existing {
		mapping[id] = pseudo
		used[pseudo] = true
		if number, ok := pseudoNumber(pseudo, prefix); ok && number >= next {
			next = number + 1
		}
	}
	fresh := make([]string, 0, len(samples))
	seen := make(map[string]bool, len(samples))
	for _, sample := range samples {
		if _, ok := mapping[sample]; ok || seen[sample] {
			continue
		}
		seen[sample] = true
		fresh = append(fresh, sample)
	}
	sort.Strings(fresh)
	width := max(minPseudonymDigits, len(strconv.Itoa(len(mapping)+len(fresh))))
	for _, sample := range fresh {
		pseudo := fmt.Sprintf("%s%0*d", prefix, width, next)
		for used[pseudo] {
			next++
			pseudo = fmt.Sprintf("%s%0*d", prefix, width, next)
		}
		next++
		mapping[sample] = pseudo
		used[pseudo] = true
	}
	return mapping
}

// pseudoNumber returns the number carried by a pseudonym such as "sample007".
func pseudoNumber(pseudo, prefix string) (int, bool) {
	digits, ok := strings.CutPrefix(pseudo, prefix)
	if !ok {
		return 0, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return number, true
}

// minPseudonymDigits is the smallest number of digits a pseudonym carries.
const minPseudonymDigits = 3

// Anonymize returns the payload with every sample id replaced by its
// pseudonym. A sample id that contains another is replaced whole.
func Anonymize(payload Payload, mapping map[string]string) Payload {
	if len(mapping) == 0 {
		return payload
	}
	ids := make([]string, 0, len(mapping))
	for id := range mapping {
		ids = append(ids, id)
	}
	// Longest first: the regex alternation then prefers the whole id.
	sort.Slice(ids, func(i, j int) bool {
		if len(ids[i]) != len(ids[j]) {
			return len(ids[i]) > len(ids[j])
		}
		return ids[i] < ids[j]
	})
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = regexp.QuoteMeta(id)
	}
	re := regexp.MustCompile(strings.Join(quoted, "|"))
	replaced, _ := replaceAll(payload, re, mapping).(Payload)
	return replaced
}

// replaceAll rewrites every string in the payload through re.
func replaceAll(node any, re *regexp.Regexp, mapping map[string]string) any {
	switch typed := node.(type) {
	case string:
		return re.ReplaceAllStringFunc(typed, func(match string) string {
			return mapping[match]
		})
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = replaceAll(v, re, mapping)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = replaceAll(v, re, mapping)
		}
		return out
	default:
		return node
	}
}

// WriteJSON writes value to path as indented JSON.
func WriteJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), fileMode)
}
