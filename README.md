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

# Only rename names matching a pattern, and rename a name the report does
# not mention at all.
replacer anonymize pipeline_info/ -o anon/ --sample-regex '^SAMP_' --extra PATIENT_07
```

`anonymize` writes `sample_map.json` next to its output: the original name and
its pseudonym. Keep that file — pass it back with `--map` to reuse the same
pseudonyms in another report, and to know what a pseudonym stands for.

### What gets renamed

The sample name is what the tool looks for, and the technical markers that
follow it are not part of it: the sheet index (`S1`), the lane (`L001`) and
the read (`R1`, `R2`). They describe the file, not the sample.

| source | example | renamed as |
| --- | --- | --- |
| the `tag` of a traced task | `TESTA_L001` | `sample001_L001` |
| the id in a `PROCESS:ALIAS (TAG)` label, in the report and the timeline | `TRIM_READS:TRIM (TESTA_L001)` | `TRIM_READS:TRIM (sample001_L001)` |
| the name inside a file name in the task script | `TESTA_S1_L001_R1_001.fastq.gz` | `sample001_S1_L001_R1_001.fastq.gz` |

Every occurrence of the name is replaced, wherever it sits in the JSON. The
lane and the read stay visible, and one sample keeps one pseudonym across
its lanes: `TESTA_L001` and `TESTA_L002` are both `sample001`.

The structure, the timings and the resource numbers are left as they are.

### What is not renamed

The payload holds no samplesheet, so a name that only shows up inside a
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
