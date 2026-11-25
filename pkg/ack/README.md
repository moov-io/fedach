# ack - FedACH FAHK Acknowledgement File Parser

This package provides a robust parser for FedACH "FAHK" acknowledgement files
(also called "Acknowledgement of ACH File Deposits" reports).

It does not look up or validate ACH routing numbers. For the FedACH
participant directory, use [moov-io/fed](https://github.com/moov-io/fed).

## Format Challenges

These files use a custom tagged format that is difficult to parse:

- The file often arrives as one or two extremely long lines (no reliable `\n`).
- Logical records are identified by a single uppercase letter prefix (`A`–`Z`).
- Many records are terminated by a `Z` character followed by padding spaces.
- Error blocks follow `I` → `J` → `K` → `Z` (file-level) and `W` → `X` → `Y` → `Z` (batch-level).
- The original content was a fixed-width visual report; newlines were stripped or became spaces.

## Primary API

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/moov-io/fedach/pkg/ack"
)

func main() {
	data, err := os.ReadFile("report.ack")
	if err != nil {
		log.Fatal(err)
	}

	recs := ack.Split(data)

	// recs is []ack.Record
	for _, r := range recs {
		fmt.Printf("[%c] %s\n", r.Prefix, string(r.Content))
	}

	// Helper to group error blocks
	fileErrs, batchErrs := ack.FindErrorBlocks(recs)
	fmt.Printf("file errors: %d, batch errors: %d\n", len(fileErrs), len(batchErrs))
	for _, block := range fileErrs {
		fmt.Println(ack.FormatErrorBlock(block))
	}
	for _, block := range batchErrs {
		fmt.Println(ack.FormatErrorBlock(block))
	}

	// File-level totals. DebitTotal and CreditTotal are integer cents.
	totals, err := ack.ParseFileTotals(recs)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("batches=%d entries=%d debit=%d credit=%d\n",
		totals.Batches, totals.Entries, totals.DebitTotal, totals.CreditTotal)

	// Optional: look up records by prefix letter
	_ = ack.RecordsByPrefix(recs)
}
```

## CLI Tool

A small command-line utility is provided at `cmd/fedach`:

```sh
# Build it (writes ./fedach in the current directory)
go build -o fedach ./cmd/fedach

# Or write it to the same path as make build
go build -o ./bin/fedach ./cmd/fedach

# Parse any .ack file (shorthand)
./fedach testdata/ack/raw/ACHFAHK....ack

# Or use the explicit subcommand
./fedach parse some-report.ack

# Show the stamped version (or "dev" if no version was stamped)
./fedach --version
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

Stage 2 (this package): take those records and parse selected contents into
structured Go types. `ParseFileTotals` returns the file-level batch and entry
counts and the debit and credit totals in cents. `FindErrorBlocks` groups
file-level (`I/J/K/Z`) and batch-level (`W/X/Y/Z`) errors, and
`FormatErrorBlock` renders a block as one readable line. `RecordsByPrefix`
indexes records by their tag letter. Further typed structs may be added over
time.

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

All eight raw files in `testdata/ack/raw/` are also exercised by the structural
tests (`TestSplit_AllRawFiles`, error-block detection, `ParseFileTotals`, etc.).
Only the files that have a manually-verified golden in `lines/` (three files)
appear in the exact-match golden table.
