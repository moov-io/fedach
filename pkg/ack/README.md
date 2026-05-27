# ack - FedACH FAHK Acknowledgement File Parser

This package provides a robust parser for FedACH "FAHK" acknowledgement files
(also called "Acknowledgement of ACH File Deposits" reports).

## Format Challenges

These files use a custom tagged format that is difficult to parse:

- The file often arrives as one or two extremely long lines (no reliable `\n`).
- Logical records are identified by a single uppercase letter prefix (`A`–`Z`).
- Many records are terminated by a `Z` character followed by padding spaces.
- Error blocks follow `I` → `J` → `K` → `Z` (file-level) and `W` → `X` → `Y` → `Z` (batch-level).
- The original content was a fixed-width visual report; newlines were stripped or became spaces.

## Primary API

```go
import "github.com/moov-io/fedach/pkg/ack"

recs := ack.Split(rawBytes)

// recs is []ack.Record
for _, r := range recs {
    fmt.Printf("[%c] %s\n", r.Prefix, string(r.Content))
}

// Helper to group error blocks
fileErrs, batchErrs := ack.FindErrorBlocks(recs)
```

## CLI Tool

A small command-line utility is provided at `cmd/fedach`:

```sh
# Build it
go build -o fedach ./cmd/fedach

# Parse any .ack file (shorthand)
./fedach testdata/ack/raw/ACHFAHK....ack

# Or use the explicit subcommand
./fedach parse some-report.ack
```

The tool inspects the file extension and dispatches to the right handler.
It shows both the reconstructed visual lines (`SplitLines`) and the tagged
records (`Split`), plus a summary of any I/J/K/Z or W/X/Y/Z error blocks.

This is the recommended way to explore new ack files during development.

## Two-Stage Parsing (Intended Usage)

Stage 1 (this package): reliably split the raw file into logical tagged records
or reconstructed visual lines (`Split` / `SplitLines`).
The `testdata/ack/lines/` directory contains verified "golden" output of
`SplitLines` for selected files in `testdata/ack/raw/`. The golden table test
uses these to ensure the extractor remains stable across changes.

Stage 2 (future): take individual records (or groups of records for error blocks)
and parse their contents into structured Go types (file totals, batch errors,
individual error messages, etc.).

## Record Content

Each `Record` contains:

- `Prefix` – the single byte `'A'..'Z'` that identifies the record type.
- `Content` – the original bytes for that logical record, including the prefix
  letter, with trailing padding whitespace trimmed.

## Testing

Run the normal test suite:

```sh
go test ./pkg/ack/...
```

### Golden Table Tests for the Line Extractor

`TestSplitLines_GoldenFiles` is a table-driven golden test. Raw inputs live in
`testdata/ack/raw/` and the corresponding expected one-logical-record-per-line
output lives in `testdata/ack/lines/` under the *exact same basename*.

To add a new regression case you only need to add a bare filename string:

```go
cases := []string{
    "ACHFAHK673960043AIN202605261654134.ack",
    "my-new-report.ack",   // drop the pair into raw/ + lines/ then add here
}
```

This structure (separate `raw/` and `lines/` subdirectories + filename-only table)
is intentional so that adding or reviewing new samples stays trivial.

All three raw files are also exercised by the structural tests
(`TestSplit_AllRawFiles`, error-block detection, etc.). Only files that have a
manually-verified golden in `lines/` appear in the exact-match golden table.
