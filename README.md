# fedach

[![Go Reference](https://pkg.go.dev/badge/github.com/moov-io/fedach.svg)](https://pkg.go.dev/github.com/moov-io/fedach)
[![Go Report Card](https://goreportcard.com/badge/github.com/moov-io/fedach)](https://goreportcard.com/report/github.com/moov-io/fedach)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Go library and CLI for parsing output files from the Federal Reserve's
[FedPayments Reporter](https://www.frbservices.org/financial-services/ach/fedpayments-reporter/)
service.

FedPayments Reporter produces operational and exception reports for ACH files
in several formats, including fixed-width visual text reports (often delivered
with newlines removed or flattened), Excel (`.xlsx`), and PDF. This module makes
those reports machine-readable and programmatically usable.

## Features

- **Robust stage-1 extractor** for noisy, concatenated, or fixed-width tagged
  report formats (no reliance on `\n` separators)
- **Two-stage architecture**: reliable logical record extraction first,
  semantic/structured parsing later
- Small, extensible **CLI** that dispatches by file extension
- Currently ships with first-class support for **FAHK / ACK**
  ("Acknowledgement of ACH File Deposits") reports via `pkg/ack`

## Installation

### CLI

```sh
# Install the latest released version
go install github.com/moov-io/fedach/cmd/fedach@latest

# Or build from source
git clone https://github.com/moov-io/fedach.git
cd fedach
make build
./bin/fedach --help
```

### Library

```sh
go get github.com/moov-io/fedach/pkg/ack
```

## Quick Start

### CLI

The CLI inspects the file extension and routes to the appropriate handler.

```sh
# Shorthand (most convenient)
./fedach testdata/ack/raw/ACHFAHK673960043AIN202605261654134.ack

# Explicit subcommand
./fedach parse some-report.ack
./fedach parse --help
```

Output includes:

- Reconstructed visual lines (what a human would have seen on screen)
- The underlying tagged logical records (`A`–`Z` + `Z` terminators)
- Summary of any file-level (`I/J/K/Z`) or batch-level (`W/X/Y/Z`) error blocks

### Go Library (ACK reports)

```go
import (
    "fmt"
    "os"

    "github.com/moov-io/fedach/pkg/ack"
)

data, _ := os.ReadFile("report.ack")

// Stage 1: extract logical records (the foundation for everything else)
recs := ack.Split(data)
lines := ack.SplitLines(data)

// recs is []ack.Record — each entry preserves the original bytes for that
// logical record (including its single-letter prefix).
for _, r := range recs {
    fmt.Printf("[%c] %s\n", r.Prefix, string(r.Content))
}

// Helper to group the two common error block patterns
fileErrs, batchErrs := ack.FindErrorBlocks(recs)
```

See the full [pkg/ack documentation](pkg/ack/README.md) for format quirks,
golden testing strategy, and the `Record` type.

## Architecture

This project follows a deliberate **two-stage model**:

1. **Stage 1 (Extraction)** — `Split` / `SplitLines` and friends reliably turn
   messy physical input (concatenated lines, embedded newlines, repeated page
   headers, glued tags, etc.) into a clean sequence of logical records or
   reconstructed visual lines. The exact original content of each record is
   preserved.
2. **Stage 2 (Semantic Parsing)** — Future packages under `pkg/` will consume
   the stage-1 output and turn it into typed Go structs (file headers, batch
   totals, error details, quoted original ACH entry data, etc.).

This separation keeps the hard low-level parsing work reusable and testable
independently of any particular report's business meaning.

New report types should follow the same pattern:
- Live under `pkg/<something>/`
- Provide their own `Split*` style extractors when needed
- The root CLI will grow a handler for the corresponding file extension

## Currently Supported

| Report | Extension | Package          | Status                  |
|--------|-----------|------------------|-------------------------|
| FAHK / ACK (Acknowledgement of ACH File Deposits) | `.ack` | `pkg/ack` | Stage 1 complete + CLI |
| Other FedPayments Reporter formats (various Excel, PDF, fixed-width) | (various) | — | Planned |

See `testdata/` for real (anonymized) sample files from the Federal Reserve.

## Development

```sh
# Run tests (includes golden regression tests for the ACK extractor)
go test ./...

# Build the CLI with a dev version stamp
make build

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
