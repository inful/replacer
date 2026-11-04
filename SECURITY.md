# Security

## Scope

`replacer` is a local command-line tool. It reads a Nextflow execution report
or timeline from disk, writes JSON next to it, and rewrites sample ids. It
opens no network connections, starts no server and runs no subprocess.

The one thing worth protecting is the data it handles: a Nextflow report
carries sample ids, file names and the paths of the work directories.

## Reporting a vulnerability

Report suspected vulnerabilities privately, not through a public issue:

```bash
gh issue create --repo inful/replacer --title "security: <summary>" --body "<details>"
```

Then ask the repository owner to convert it into a private advisory, or use
GitHub's private vulnerability reporting if it is enabled for this repository.

Please include the version (`replacer version`), the platform, and the steps
needed to reproduce.

## Handling reports

The tool writes what it is given. Sample ids are renamed only when it is told
to: check the `sample_map.json` it writes, and treat that file as sensitive as
the report it came from — it maps every pseudonym back to a real sample id.

To share a report outside the team, anonymize it first and read the output
before sending it:

```bash
replacer anonymize pipeline_info/ -o anon/ --derive-prefix
```
