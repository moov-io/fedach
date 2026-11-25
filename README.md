# fedach

[![Go Reference](https://pkg.go.dev/badge/github.com/moov-io/fedach.svg)](https://pkg.go.dev/github.com/moov-io/fedach)
[![Go Report Card](https://goreportcard.com/badge/github.com/moov-io/fedach)](https://goreportcard.com/report/github.com/moov-io/fedach)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Go library and CLI for parsing FedPayments Reporter FAHK/ACK
("Acknowledgement of ACH File Deposits") reports from the Federal Reserve's
[FedPayments Reporter](https://www.frbservices.org/financial-services/ach/fedpayments-reporter/)
service.

This project does not look up or validate ACH routing numbers, and it does not
parse the FedACH participant directory. For routing-number and bank-name
lookup, use [moov-io/fed](https://github.com/moov-io/fed). For ABA check-digit
helpers, see [moov-io/ach](https://github.com/moov-io/ach).

FedPayments Reporter produces operational and exception reports for ACH files
in several formats, including fixed-width visual text reports (often delivered
with newlines removed or flattened), Excel (`.xlsx`), and PDF. This module
makes FAHK/ACK reports machine-readable and programmatically usable. Support
for the other report formats is planned.

## Features

- **Robust stage-1 extractor** for noisy, concatenated, or fixed-width tagged
  report formats (no reliance on `\n` separators)
- **Two-stage architecture**: reliable logical record extraction first,
  then typed helpers for file totals and error blocks
- Small, extensible **CLI** that dispatches by file extension
- First-class support for **FAHK / ACK** reports via `pkg/ack`, including
  `ParseFileTotals`, `FindErrorBlocks`, `FormatErrorBlock`, and
  `RecordsByPrefix`

## Prerequisites

- Go 1.26.3 or later
- `make`, `git`, and `wget` if you use the `make build` or `make check` targets
  (`go build` and `go test` do not need them)

## Installation

### CLI

```sh
# Install the latest released version
go install github.com/moov-io/fedach/cmd/fedach@latest

# Or build from source
git clone https://github.com/moov-io/fedach.git
cd fedach
go build -o ./bin/fedach ./cmd/fedach
./bin/fedach --help

# Optional: stamp a version with make (requires make, git, and wget)
make build
./bin/fedach --version
```

After `go install`, the binary is on your `PATH` as `fedach` (in `$GOBIN` or
`$(go env GOPATH)/bin`). After `go build` or `make build`, the binary is
`./bin/fedach`.

### Library

```sh
go get github.com/moov-io/fedach/pkg/ack
```

## Quick Start

### CLI

The CLI inspects the file extension and routes to the appropriate handler.

```sh
# After go install (binary is named fedach)
fedach testdata/ack/raw/ACHFAHK673960043AIN202605261654134.ack

# After go build -o ./bin/fedach ./cmd/fedach (or make build)
./bin/fedach testdata/ack/raw/ACHFAHK673960043AIN202605261654134.ack

# Explicit subcommand
./bin/fedach parse some-report.ack
./bin/fedach parse --help
```

Output includes:

- Reconstructed visual lines (what a human would have seen on screen)
- The underlying tagged logical records (`A`–`Z` + `Z` terminators)
- Summary of any file-level (`I/J/K/Z`) or batch-level (`W/X/Y/Z`) error blocks

### Go Library (ACK reports)

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

	// Stage 1: extract logical records (the foundation for everything else)
	recs := ack.Split(data)

	// recs is []ack.Record — each entry preserves the original bytes for that
	// logical record (including its single-letter prefix).
	for _, r := range recs {
		fmt.Printf("[%c] %s\n", r.Prefix, string(r.Content))
	}

	// Group the two common error block patterns and print a readable summary
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

	// Optional: look up records by prefix letter (A, I, W, ...)
	_ = ack.RecordsByPrefix(recs)
}
```

See the full [pkg/ack documentation](pkg/ack/README.md) and the package
[example test](pkg/ack/example_test.go) for format quirks, golden testing
strategy, and the `Record` type.

## Architecture

This project follows a deliberate **two-stage model**:

1. **Stage 1 (Extraction)** — `Split` / `SplitLines` and friends reliably turn
   messy physical input (concatenated lines, embedded newlines, repeated page
   headers, glued tags, etc.) into a clean sequence of logical records or
   reconstructed visual lines. The exact original content of each record is
   preserved.
2. **Stage 2 (Semantic Parsing)** — Helpers under `pkg/ack` consume the
   stage-1 records and turn them into typed values. `ParseFileTotals` returns
   batch and entry counts plus debit and credit totals in cents.
   `FindErrorBlocks` and `FormatErrorBlock` group and render file-level
   (`I/J/K/Z`) and batch-level (`W/X/Y/Z`) errors. `RecordsByPrefix` indexes
   records by their tag letter. Further typed structs (quoted original ACH
   entry data and other report kinds) may be added over time.

This separation keeps the hard low-level parsing work reusable and testable
independently of any particular report's business meaning.

New report types should follow the same pattern:
- Live under `pkg/<something>/`
- Provide their own `Split*` style extractors when needed
- The root CLI will grow a handler for the corresponding file extension

## Currently Supported

| Report | Extension | Package          | Status                  |
|--------|-----------|------------------|-------------------------|
| FAHK / ACK (Acknowledgement of ACH File Deposits) | `.ack` | `pkg/ack` | Stage 1 complete, file totals and error helpers, CLI |
| Other FedPayments Reporter formats (various Excel, PDF, fixed-width) | (various) | — | Planned |

## Sample files and reference documents

See [`testdata/`](testdata/) for real (anonymized) sample files from the
Federal Reserve, and [`docs/`](docs/) for reference PDFs.

- [`testdata/ack/raw/`](testdata/ack/raw/) — FAHK/ACK samples, including
  flattened production files and newline-formatted Federal Reserve
  documentation examples. These are the files the CLI and `pkg/ack` parse
  today.
- Excel (`.xlsx`) and PDF samples in `testdata/` (originated entries, received
  entries, return reason, configuration, and the 2024 sample-report deck) are
  included for planned parsers. The CLI reports
  `unsupported file extension ".xlsx"` (and the same for `.pdf`) because those
  handlers are not implemented yet.
- [`docs/FedACH_CIPS_Requirements.pdf`](docs/FedACH_CIPS_Requirements.pdf) —
  FedACH Computer Interface Protocol Specifications. The newline-formatted
  ACK examples in `testdata/ack/raw/` come from this document.
- [`docs/fedach-platform-modernization-webinar.pdf`](docs/fedach-platform-modernization-webinar.pdf) —
  Federal Reserve FedACH Platform Modernization webinar slides.

Package documentation: [pkg.go.dev/github.com/moov-io/fedach](https://pkg.go.dev/github.com/moov-io/fedach).

## Development

```sh
# Run tests (includes golden regression tests for the ACK extractor)
go test ./...

# Build the CLI with a dev version stamp
make build
./bin/fedach --version

# Full project checks (linting, coverage, etc.)
make check
```

The ACK package uses a "just a filename" golden table test pattern. Raw inputs
live in `testdata/ack/raw/` and the corresponding expected line-by-line output
lives in `testdata/ack/lines/` under the exact same basename. Adding a new
regression case is as simple as dropping the pair of files and adding the name
to the test slice.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Acknowledgements

This project is part of the [moov-io](https://github.com/moov-io) family of
financial infrastructure libraries. Special thanks to the Federal Reserve
Banks for publishing the FedPayments Reporter service and its sample reports.
