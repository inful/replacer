// Command replacer pulls the machine-readable JSON out of a Nextflow
// execution report and timeline, and anonymizes the sample ids in it.
//
//	replacer extract pipeline_info/ -o json/
//	replacer anonymize pipeline_info/ -o anon/ --sample-regex '^SAMP_'
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/inful/replacer"
)

// Exit codes are semantic so a shell script can branch on the outcome.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// version is overridden at build time via -ldflags.
var version = "dev"

// File modes for the files and directories the tool writes.
const (
	dirMode  = 0o750
	fileMode = 0o600
)

type cmdExtract struct {
	Inputs []string `arg:""                                                 help:"Report or timeline HTML files, or directories holding them." name:"inputs" optional:"" type:"path"`
	Output string   `help:"Output directory (default: next to each input)." short:"o"                                                          type:"path"`
}

type cmdAnonymize struct {
	Inputs      []string `arg:""                                                                      help:"Report or timeline HTML or JSON files, or directories holding them." name:"inputs" optional:"" type:"path"`
	Output      string   `help:"Output directory (default: next to each input)."                      short:"o"                                                                  type:"path"`
	Map         string   `help:"Sample map file: read when it exists, written with the full mapping." type:"path"`
	SampleRegex string   `help:"Only rename sample names matching this regex."                        name:"sample-regex"`
	Extra       []string `help:"Additional sample name to rename (repeatable)."`
	Prefix      string   `default:"sample"                                                            help:"Pseudonym prefix."`
}

type cmdVersion struct{}

type cli struct {
	Extract   cmdExtract   `cmd:"" help:"Write the embedded JSON of each report or timeline HTML file."`
	Anonymize cmdAnonymize `cmd:"" help:"Rename the sample ids in report or timeline JSON."`
	Version   cmdVersion   `cmd:"" help:"Print the version and exit."`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main's body, parameterised on its IO so tests can drive it
// without touching the process environment.
func run(args []string, stdout, stderr io.Writer) int {
	var c cli
	parser := kong.Must(&c,
		kong.Name("replacer"),
		kong.Description("Extract and anonymize Nextflow execution report and timeline JSON."),
		kong.Exit(func(int) {}),
		kong.Vars{"version": version},
	)
	ctx, err := parser.Parse(args)
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitUsage
	}

	// Kong reports the selected command including its positional
	// arguments, e.g. "extract <inputs>" rather than plain "extract".
	name, _, _ := strings.Cut(ctx.Command(), " ")
	switch name {
	case "extract":
		return runExtract(c.Extract, stdout, stderr)
	case "anonymize":
		return runAnonymize(c.Anonymize, stdout, stderr)
	case "version":
		printf(stdout, "replacer %s\n", buildVersion())
		return exitOK
	default:
		printf(stderr, "replacer: unknown command %q\n", ctx.Command())
		return exitUsage
	}
}

// printf writes to w, ignoring the error. Every use is diagnostic output on
// a stream the caller already owns; there is nothing useful to do if stderr
// itself has failed.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// buildVersion returns the version of the running binary: the one GoReleaser
// injects, or the module version recorded in the build info when the binary
// was installed with "go install" — that build path carries no ldflags.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

// runExtract writes the JSON payload of every input file next to it.
func runExtract(cmd cmdExtract, stdout, stderr io.Writer) int {
	files, err := collectFiles(cmd.Inputs, ".html", ".htm")
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}
	if len(files) == 0 {
		printf(stderr, "replacer: no HTML files found\n")
		return exitError
	}
	for _, file := range files {
		payload, err := replacer.ExtractPayload(file)
		if err != nil {
			printf(stderr, "replacer: %v\n", err)
			return exitError
		}
		target, err := outputPath(cmd.Output, file, ".json")
		if err != nil {
			printf(stderr, "replacer: %v\n", err)
			return exitError
		}
		if err := replacer.WriteJSON(target, payload); err != nil {
			printf(stderr, "replacer: %v\n", err)
			return exitError
		}
		printf(stdout, "%s -> %s\n", file, target)
	}
	return exitOK
}

// runAnonymize renames the sample ids of every input file and records the
// mapping so the same pseudonyms can be reused.
func runAnonymize(cmd cmdAnonymize, stdout, stderr io.Writer) int {
	files, err := collectFiles(cmd.Inputs, ".html", ".htm", ".json")
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}
	if len(files) == 0 {
		printf(stderr, "replacer: no input files found\n")
		return exitError
	}

	payloads, err := loadPayloads(files)
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}

	samples, err := selectSamples(cmd, payloads)
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitUsage
	}
	if len(samples) == 0 {
		printf(stderr, "replacer: warning: no sample id detected, nothing renamed\n")
	}

	mapPath := sampleMapPath(cmd, files)
	existing, err := readMap(mapPath)
	if err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}
	mapping := replacer.Mapping(samples, existing, cmd.Prefix)

	if err := writeAnonymized(cmd, files, payloads, mapping, stdout); err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}
	if err := replacer.WriteJSON(mapPath, mapping); err != nil {
		printf(stderr, "replacer: %v\n", err)
		return exitError
	}
	printf(stdout, "sample map -> %s\n", mapPath)
	printf(stdout, "%d sample name(s) renamed\n", len(mapping))
	return exitOK
}

// loadPayloads reads the payload of every file.
func loadPayloads(files []string) ([]replacer.Payload, error) {
	payloads := make([]replacer.Payload, 0, len(files))
	for _, file := range files {
		payload, err := replacer.LoadPayload(file)
		if err != nil {
			return nil, err
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

// writeAnonymized writes the anonymized payload of every file.
func writeAnonymized(cmd cmdAnonymize, files []string, payloads []replacer.Payload, mapping map[string]string, stdout io.Writer) error {
	for i, file := range files {
		target, err := outputPath(cmd.Output, file, ".anonymized.json")
		if err != nil {
			return err
		}
		anonymized := replacer.Anonymize(payloads[i], mapping)
		if err := replacer.WriteJSON(target, anonymized); err != nil {
			return err
		}
		printf(stdout, "%s -> %s\n", file, target)
	}
	return nil
}

// selectSamples returns the sample names to rename: the names found in the
// payloads, filtered by --sample-regex, plus the names named on the command
// line.
func selectSamples(cmd cmdAnonymize, payloads []replacer.Payload) ([]string, error) {
	samples := replacer.Samples(payloads)
	if cmd.SampleRegex != "" {
		pattern, err := regexp.Compile(cmd.SampleRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid --sample-regex: %w", err)
		}
		kept := make([]string, 0, len(samples))
		for _, sample := range samples {
			if pattern.MatchString(sample) {
				kept = append(kept, sample)
			}
		}
		samples = kept
	}
	for _, extra := range cmd.Extra {
		samples = append(samples, replacer.SampleName(extra))
	}
	return samples, nil
}

// sampleMapPath returns the path of the sample map file.
func sampleMapPath(cmd cmdAnonymize, files []string) string {
	if cmd.Map != "" {
		return cmd.Map
	}
	return filepath.Join(outputDir(cmd.Output, files[0]), "sample_map.json")
}

// readMap reads an existing sample map. A missing file is an empty map.
func readMap(path string) (map[string]string, error) {
	existing := map[string]string{}
	raw, err := os.ReadFile(path) //nolint:gosec // the path is the user's own input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return existing, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return existing, nil
}

// collectFiles expands directories into the files with a wanted suffix.
func collectFiles(inputs []string, suffixes ...string) ([]string, error) {
	var files []string
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, input)
			continue
		}
		entries, err := os.ReadDir(input)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !hasSuffix(entry.Name(), suffixes) {
				continue
			}
			files = append(files, filepath.Join(input, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// hasSuffix reports whether name ends with one of the suffixes, ignoring case.
func hasSuffix(name string, suffixes []string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return slices.Contains(suffixes, ext)
}

// outputDir returns the directory the outputs of file belong in.
func outputDir(output, file string) string {
	if output != "" {
		return output
	}
	return filepath.Dir(file)
}

// outputPath returns the path of the output file for an input file and
// creates the directory it goes in.
func outputPath(output, file, suffix string) (string, error) {
	dir := outputDir(output, file)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	stem = strings.TrimSuffix(stem, ".anonymized")
	return filepath.Join(dir, stem+suffix), nil
}
