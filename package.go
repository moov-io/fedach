// Package fedach provides Go parsers, utilities, and a CLI for working with
// the various report files produced by the Federal Reserve Banks'
// FedPayments Reporter service.
//
// FedPayments Reporter (https://www.frbservices.org/financial-services/ach/fedpayments-reporter/)
// generates operational and exception reports for ACH files processed by
// the Federal Reserve. These reports are delivered in several formats,
// including fixed-width visual text reports (often with newlines removed or
// flattened), Excel (.xlsx), and PDF.
//
// This module focuses on making those reports machine-readable and
// programmatically usable. The design follows a two-stage model:
//
//  1. Reliable extraction of logical records / visual lines from noisy,
//     concatenated, or fixed-width input (the "stage 1" parser).
//  2. Semantic parsing of those records into structured Go types for
//     specific report kinds (the "stage 2" parsers, added over time).
//
// # Currently Supported Reports
//
//   - FAHK / ACK files ("Acknowledgement of ACH File Deposits") via
//     the [github.com/moov-io/fedach/pkg/ack] subpackage.
//     These use an in-band tagged format with single-letter prefixes
//     (A–Z) and explicit 'Z' terminators. The ack package provides
//     Split, SplitLines, Record, and FindErrorBlocks helpers plus
//     robust reconstruction of the original visual report layout.
//
// # Command-Line Tool
//
// A small, extensible CLI lives in cmd/fedach. It dispatches on file
// extension so it can grow to support additional report types:
//
//	go run ./cmd/fedach parse path/to/report.ack
//	./fedach somefile.ack
//
// The CLI currently understands .ack files and will be extended for
// other FedPayments Reporter outputs.
//
// # Adding New Report Types
//
// New report formats should live under pkg/ (e.g. pkg/returns,
// pkg/originated) following the same two-stage extraction + semantic
// model. The root CLI will route based on file extension.
//
// For sample input files see the testdata/ directory, which contains
// real (anonymized) reports from the Federal Reserve.
package fedach
