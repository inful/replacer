# replacer

[![CI](https://github.com/inful/replacer/actions/workflows/ci.yml/badge.svg)](https://github.com/inful/replacer/actions/workflows/ci.yml)
[![Release](https://github.com/inful/replacer/actions/workflows/release.yml/badge.svg)](https://github.com/inful/replacer/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/inful/replacer)](https://goreportcard.com/report/github.com/inful/replacer)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

Pull the machine-readable JSON out of a Nextflow execution report and
timeline, and anonymize the sample ids in it.

Nextflow renders `pipeline_info/execution_report.html` and
`pipeline_info/execution_timeline.html` from templates that assign the data
to a global, so the JSON is in the file, not just in the page it draws:

```html
window.data = { "trace": [...], "summary": [...] };   <!-- execution_report.html -->
window.data = { "elapsed": "...", "processes": [...] }; <!-- execution_timeline.html -->
```

`replacer` reads that payload, writes it as plain JSON, and renames the
sample ids it finds — in the task tag, in the `PROCESS:ALIAS (TAG)` label and
inside the task script, where file names are built from the sample id.

## Install

```bash
go install github.com/inful/replacer/cmd/replacer@latest
```

Or download a binary for your platform from the
[releases page](https://github.com/inful/replacer/releases):

```bash
tar -xzf replacer_<version>_<os>_<arch>.tar.gz
```

Or build from a checkout:

```bash
go build -o replacer ./cmd/replacer
```

## Use

```bash
# Write the embedded JSON next to the HTML files.
replacer extract pipeline_info/ -o json/

# Write anonymized JSON: sample ids become sample001, sample002, ...
replacer anonymize pipeline_info/ -o anon/

# Same, but also rename the bare sample name a tag is built from
# (TESTA_L002 also renames TESTA, which appears in file names).
replacer anonymize pipeline_info/ -o anon/ --derive-prefix

# Only rename ids matching a pattern, and rename an id the report does not
# mention at all.
replacer anonymize pipeline_info/ -o anon/ --sample-regex '^SAMP_' --extra PATIENT_07
```

`anonymize` writes `sample_map.json` next to its output: the original id and
its pseudonym. Keep that file — pass it back with `--map` to reuse the same
pseudonyms in another report, and to know what a pseudonym stands for.

### What gets renamed

Sample ids are found in three places:

| source | example |
| --- | --- |
| the `tag` of a traced task | `TESTA_L002` |
| the id in a `PROCESS:ALIAS (TAG)` label, in the report and the timeline | `TRIM_READS:TRIM (TESTA_L002)` |
| the bare name a tag is built from, with `--derive-prefix` | `TESTA` |

Every occurrence of those ids is replaced, wherever it sits in the JSON.
The structure, the timings and the resource numbers are left as they are.

### What is not renamed

The payload holds no samplesheet, so an id that only shows up inside a
derived file name is invisible to the tool — pass it with `--extra`. Paths
such as the task `workdir` keep their shape; they carry hashes, not names.

## Exit codes

| code | meaning |
| --- | --- |
| 0 | success |
| 1 | error (unreadable input, no payload, no sample ids) |
| 2 | usage error |

## Development

```bash
go test -race -count=1 ./...
golangci-lint run ./...
goreleaser release --snapshot --clean --skip=publish
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the gates, the commit conventions
and the release process.

## License

GPL-3.0 — see [LICENSE](LICENSE).
