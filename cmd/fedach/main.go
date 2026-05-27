// Package main implements the fedach command-line tool.
//
// It provides utilities for inspecting and parsing various FedACH /
// FedPayments Reporter output files. The tool dispatches based on file
// extension so it can grow to support .ack, .xlsx, PDF, and other report
// formats over time.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	fedach "github.com/moov-io/fedach"
	"github.com/urfave/cli/v3"

	"github.com/moov-io/fedach/pkg/ack"
)

func main() {
	root := &cli.Command{
		Name:    "fedach",
		Usage:   "FedACH and FedPayments Reporter file inspection tools",
		Version: fedach.Version,
		Description: `fedach is a Swiss Army knife for the various report files produced
by the Federal Reserve's FedPayments Reporter service.

It inspects the file extension and routes to the appropriate parser
(ack is the first supported format).`,
		Commands: []*cli.Command{
			parseCommand(),
		},
		// Allow `fedach somefile.ack` as a convenient shorthand for
		// `fedach parse somefile.ack` when the argument is a file we support.
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() > 0 {
				f := cmd.Args().First()
				// Only treat it as a file if it looks like one that exists
				// or has a known extension. Otherwise fall through to help.
				if _, err := os.Stat(f); err == nil || hasKnownExtension(f) {
					return runParse(f)
				}
			}
			return cli.ShowAppHelp(cmd)
		},
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func parseCommand() *cli.Command {
	return &cli.Command{
		Name:      "parse",
		Aliases:   []string{"p"},
		Usage:     "Parse a report file and display its logical structure",
		ArgsUsage: "<file>",
		Description: `Reads the given report file, determines its type from the
file extension, and prints a human-readable view of the logical
records / lines it contains.

Currently supported extensions:
  .ack   - FedACH FAHK Acknowledgement of ACH File Deposits reports`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return fmt.Errorf("parse requires a file argument (e.g. fedach parse report.ack)")
			}
			return runParse(cmd.Args().First())
		},
	}
}

// hasKnownExtension reports whether the path has an extension we can
// eventually handle. Used by the root action for the convenient shorthand.
func hasKnownExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ack":
		return true
	// Add more as support is implemented:
	// case ".xlsx", ".xls":
	//     return true
	default:
		return false
	}
}

// runParse is the core dispatcher. It inspects the file extension first
// (so we give a clear "unsupported type" error even for nonexistent files),
// then reads the bytes and delegates to the type-specific handler.
func runParse(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return fmt.Errorf("file %q has no extension; cannot determine report type", path)
	}

	var handler func([]byte, string) error
	switch ext {
	case ".ack":
		handler = handleAckReport
	// Future handlers (examples of how to grow support):
	// case ".xlsx", ".xls":
	//     handler = handleExcelReport
	// case ".pdf":
	//     handler = handlePDFReport
	default:
		return fmt.Errorf("unsupported file extension %q (supported: .ack)", ext)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	return handler(data, path)
}

// handleAckReport knows how to pretty-print the contents of a FAHK .ack file.
func handleAckReport(data []byte, path string) error {
	// Two views are useful:
	//   1. SplitLines — the reconstructed visual report lines (what a human
	//      would have seen on screen or in the PDF/Excel version).
	//   2. Split — the individual tagged logical records (A/B/C/I/J/K/W/X/Y/Z etc.)
	//      which are the foundation for future semantic parsing.
	lines := ack.SplitLines(data)
	recs := ack.Split(data)

	fmt.Printf("File:     %s\n", path)
	fmt.Printf("Type:     ACK (FAHK - Acknowledgement of ACH File Deposits)\n")
	fmt.Printf("Size:     %d bytes\n", len(data))
	fmt.Printf("Records:  %d tagged logical records\n", len(recs))
	fmt.Printf("Lines:    %d reconstructed visual lines\n\n", len(lines))

	// Show the reconstructed visual report first — this is the most
	// immediately useful output for a human.
	fmt.Println("=== Reconstructed Visual Lines ===")
	for i, line := range lines {
		fmt.Printf("%3d: %s\n", i+1, line)
	}

	// Also show a compact summary of the tagged records (helpful for
	// developers working on stage-2 semantic parsers).
	fmt.Println("\n=== Tagged Logical Records (first 20) ===")
	limit := len(recs)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		r := recs[i]
		preview := string(r.Content)
		if len(preview) > 70 {
			preview = preview[:67] + "..."
		}
		fmt.Printf("  [%c] %s\n", r.Prefix, preview)
	}
	if len(recs) > 20 {
		fmt.Printf("  ... (%d more records)\n", len(recs)-20)
	}

	// If there were error blocks, surface them prominently.
	fileErrs, batchErrs := ack.FindErrorBlocks(recs)
	if len(fileErrs) > 0 || len(batchErrs) > 0 {
		fmt.Printf("\n=== Error Blocks Detected ===\n")
		fmt.Printf("  File-level errors (I/J/K/Z): %d\n", len(fileErrs))
		fmt.Printf("  Batch-level errors (W/X/Y/Z): %d\n", len(batchErrs))
	}

	return nil
}
