package ack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The two files in testdata/ack/raw/ (file-level-example and
// file-multi-batch-level-example) are taken directly from the Federal
// Reserve's official FAHK documentation. They use real newlines and a
// "pretty" presentation rather than the flattened single-line format
// that production ack files arrive in. The parser still processes them
// (newlines become spaces), and these tests lock in the current behavior
// with exact literal expectations.

func TestExamples_FileLevelExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ack", "raw", "file-level-example.ack"))
	require.NoError(t, err)

	// Split produces a single giant record because the input uses
	// newlines and the internal heuristics are tuned for flattened
	// production files. We still assert the observable facts exactly.
	recs := Split(raw)
	require.Len(t, recs, 1)
	require.Equal(t, byte('A'), recs[0].Prefix)
	require.NotEmpty(t, recs[0].Content)
	require.Equal(t, byte('A'), recs[0].Content[0])
	// Key phrases from the official example must be present inside the
	// collapsed content.
	content := string(recs[0].Content)
	require.Contains(t, content, "IFH238-INVALID IMMEDIATE ORIGIN NOT AUTHORIZED AS A SENDING POINT")
	require.Contains(t, content, "IAJ095-EXCEEDED ALLOWED NUMBER OF ITEM REJECTS PER FILE")
	require.Contains(t, content, "BANK OF TEST")
	require.Contains(t, content, "FILE ACCEPTED WITH NO ERRORS")

	// SplitLines does a much better job recovering the visual report
	// structure even on these newline-rich official examples.
	lines := SplitLines(raw)
	// 81 lines: the pretty-printed doc example has one physical line per visual
	// row (including many Z spacers and trailing Z's). The dedicated pretty-path
	// in SplitLines splits on the original \n (after stripping the Fed's 1-9/A-Z
	// line-type indicators and Z terminators) to reconstruct the exact report
	// appearance from FedACH_CIPS_Requirements.pdf pp.21-22.
	require.Len(t, lines, 81)
	require.Equal(t, "AJ001A01A08052", lines[0])
	require.Equal(t, "****** ACKNOWLEDGEMENT OF ACH FILE DEPOSITS ******", lines[1])
	require.Equal(t, "SERVICING FRB: FEDERAL RESERVE BANK REPORT DATE AND TIME:", lines[2])
	require.Equal(t, "07/20/20 09:30:24", lines[3])
	require.Equal(t, "FILE STATUS: FILE REJECTED WITH FILE LEVEL ERRORS", lines[4])
	require.Equal(t, "IMMEDIATE ORIGIN NAME: BANK OF TEST", lines[6])
	require.Equal(t, "IMMEDIATE ORIGIN: 1234-5678-9", lines[7])
	require.Equal(t, " BATCHES: 10,081 DEBIT: $ 7,391,935.03", lines[16])
	require.Equal(t, "FILE STATUS: FILE REJECTED WITH FILE LEVEL ERRORS", lines[21])
	require.Equal(t, "FH238-INVALID IMMEDIATE ORIGIN NOT AUTHORIZED AS A SENDING POINT", lines[23])
	require.Equal(t, "IMMEDIATE ORIGIN = 123456789", lines[24])
	require.Equal(t, "AJ095-EXCEEDED ALLOWED NUMBER OF ITEM REJECTS PER FILE", lines[27])
	require.Equal(t, "************************** END OF ACKNOWLEDGEMENT *************************", lines[30])
	// Second page (non-initial header) per p.7-8 of the PDF.
	require.Equal(t, "AJ001A01A08052", lines[54])
	require.Equal(t, "FILE STATUS: FILE ACCEPTED WITH NO ERRORS", lines[58])
	require.Equal(t, " BATCHES: 3 DEBIT: $ 0.00", lines[70])
	require.Equal(t, "************************** END OF ACKNOWLEDGEMENT *************************", lines[80])

	// Error block detection finds nothing because the I/J/K letters
	// were never promoted to top-level Records by Split on this input.
	fileErrs, batchErrs := FindErrorBlocks(recs)
	require.Equal(t, 0, len(fileErrs))
	require.Equal(t, 0, len(batchErrs))

	// RecordsByPrefix
	byPrefix := RecordsByPrefix(recs)
	require.Len(t, byPrefix, 1)
	require.Len(t, byPrefix['A'], 1)

	// FormatErrorBlock on empty inputs
	require.Equal(t, "", FormatErrorBlock(nil))
	require.Equal(t, "", FormatErrorBlock([]Record{}))
}

func TestExamples_FileMultiBatchLevelExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ack", "raw", "file-multi-batch-level-example.ack"))
	require.NoError(t, err)

	recs := Split(raw)
	require.Len(t, recs, 1)
	require.Equal(t, byte('A'), recs[0].Prefix)
	content := string(recs[0].Content)
	require.Contains(t, content, "IBH501-ORIGINATING DFI CANNOT ORIGINATE IN CONTROLLED TEST")
	require.Contains(t, content, "WBH232-INVALID ORIGINATING DFI IDENTIFICATION NOT AUTHORIZED TO ORIGINATE")
	require.Contains(t, content, "APPLICATION SUPERVISOR")
	require.Contains(t, content, "BATCH REJECTED")

	lines := SplitLines(raw)
	// 113 lines because the pretty-printed "File/MultiBatch Level Example" from
	// FedACH_CIPS_Requirements.pdf (pp.23-25) has one \n per visual row. The
	// dedicated pretty-path splits faithfully on those newlines (after removing
	// the 1-9/A-Z line-type indicators and Z terminators), so the $32k summary,
	// the 4 IBH501 controlled-test file-level errors, the L spacer, the per-batch
	// M/N/O/P/Q/R/S/T/U/V details, the page breaks (AJ001.. + Z), and the WBH232
	// batch-level errors all appear as separate short lines instead of one giant
	// concatenation.
	require.Len(t, lines, 113)
	require.Equal(t, "AJ001A01A08052", lines[0])
	require.Equal(t, "****** ACKNOWLEDGEMENT OF ACH FILE DEPOSITS ******", lines[1])
	require.Equal(t, "SERVICING FRB: FEDERAL RESERVE BANK REPORT DATE AND TIME:", lines[2])
	require.Equal(t, "07/20/20 12:02:24", lines[3])
	require.Equal(t, "FILE STATUS: FILE REJECTED WITH FILE LEVEL ERRORS", lines[4])
	require.Equal(t, "IMMEDIATE ORIGIN NAME: APPLICATION SUPERVISOR", lines[6])
	require.Equal(t, " BATCHES: 4 DEBIT: $ 32,960.34", lines[16])
	require.Equal(t, "ENTRY/ADDENDA: 396 CREDITS: $ 80,038.06", lines[17])
	// The G row + four IBH/J/K error groups (each now separate lines, plus
	// spacer "" lines from H/K/Z). This is the area that used to collapse into
	// the giant lines[11] string the user noted.
	require.Equal(t, "FILE STATUS: FILE REJECTED WITH FILE LEVEL ERRORS", lines[21])
	require.Equal(t, "BH501-ORIGINATING DFI CANNOT ORIGINATE IN CONTROLLED TEST", lines[23])
	require.Equal(t, "ORIGINATION DFI 69100013 FOR BATCH 0000001", lines[24])
	require.Equal(t, "BH501-ORIGINATING DFI CANNOT ORIGINATE IN CONTROLLED TEST", lines[27])
	require.Equal(t, "ORIGINATION DFI 69100013 FOR BATCH 0000004", lines[28])
	require.Equal(t, "BH501-ORIGINATING DFI CANNOT ORIGINATE IN CONTROLLED TEST", lines[31])
	require.Equal(t, "ORIGINATION DFI 69100013 FOR BATCH 0000002", lines[32])
	require.Equal(t, "BH501-ORIGINATING DFI CANNOT ORIGINATE IN CONTROLLED TEST", lines[35])
	require.Equal(t, "ORIGINATION DFI 69100013 FOR BATCH 0000003", lines[36])
	require.Equal(t, "BATCH NUMBER: 0000001", lines[40])
	require.Equal(t, " ORIGINATING DFI ID: 6910-0013-4", lines[41])
	require.Equal(t, "BATCH STATUS: BATCH REJECTED", lines[51])
	// Second page header (with its following Z line per PDF p.7-8) then the
	// WBH232 batch errors + remaining per-batch details for batches 1-4.
	require.Equal(t, "AJ001A01A08052", lines[53])
	require.Equal(t, "BH232-INVALID ORIGINATING DFI IDENTIFICATION NOT AUTHORIZED TO ORIGINATE", lines[55])
	require.Equal(t, "ORIGINATING DFI IDENTIFICATION = 691000134 IN BATCH 0000001", lines[56])
	require.Equal(t, "BATCH STATUS: BATCH REJECTED", lines[104])
	require.Equal(t, "AJ001A01A08052", lines[107])
	require.Equal(t, "************************** END OF ACKNOWLEDGEMENT *************************", lines[112])

	fileErrs, batchErrs := FindErrorBlocks(recs)
	require.Equal(t, 0, len(fileErrs))
	require.Equal(t, 0, len(batchErrs))

	byPrefix := RecordsByPrefix(recs)
	require.Len(t, byPrefix, 1)
	require.Len(t, byPrefix['A'], 1)

	require.Equal(t, "", FormatErrorBlock(nil))
	require.Equal(t, "", FormatErrorBlock([]Record{}))
}

// TestExamples_FormatErrorBlock exercises FormatErrorBlock with literal
// expectations using small hand-constructed blocks (the two official
// doc files above do not yield extractable error blocks with the
// current Split heuristics).
func TestExamples_FormatErrorBlock(t *testing.T) {
	// Simple file-level error block (I + J + K + Z)
	block := []Record{
		{Prefix: 'I', Content: []byte("IFH238-INVALID IMMEDIATE ORIGIN NOT AUTHORIZED AS A SENDING POINT")},
		{Prefix: 'J', Content: []byte("JIMMEDIATE ORIGIN = 123456789")},
		{Prefix: 'K', Content: []byte("K")},
		{Prefix: 'Z', Content: []byte("Z")},
	}
	require.Equal(t,
		"FH238-INVALID IMMEDIATE ORIGIN NOT AUTHORIZED AS A SENDING POINT IMMEDIATE ORIGIN = 123456789",
		FormatErrorBlock(block))

	// Batch error block with real Fed error code
	batchBlock := []Record{
		{Prefix: 'W', Content: []byte("WBH232-INVALID ORIGINATING DFI IDENTIFICATION NOT AUTHORIZED TO ORIGINATE")},
		{Prefix: 'X', Content: []byte("XORIGINATING DFI IDENTIFICATION = 691000134 IN BATCH 0000001")},
		{Prefix: 'Y', Content: []byte("Y")},
		{Prefix: 'Z', Content: []byte("Z")},
	}
	require.Equal(t,
		"BH232-INVALID ORIGINATING DFI IDENTIFICATION NOT AUTHORIZED TO ORIGINATE ORIGINATING DFI IDENTIFICATION = 691000134 IN BATCH 0000001",
		FormatErrorBlock(batchBlock))

	// Continuation records that legitimately start with "ID" / "IN" must keep the prefix letter
	keepID := []Record{
		{Prefix: 'I', Content: []byte("ITH999-SOME ERROR")},
		{Prefix: 'J', Content: []byte("JID = THE ID VALUE")},
	}
	require.Equal(t, "TH999-SOME ERROR ID = THE ID VALUE", FormatErrorBlock(keepID))

	keepIN := []Record{
		{Prefix: 'I', Content: []byte("ITH888-ANOTHER")},
		{Prefix: 'J', Content: []byte("JINCOMING VALUE = 42")},
	}
	require.Equal(t, "TH888-ANOTHER INCOMING VALUE = 42", FormatErrorBlock(keepIN))

	// Empty and nil cases already covered in the per-file tests, but double-check here
	require.Equal(t, "", FormatErrorBlock(nil))
	require.Equal(t, "", FormatErrorBlock([]Record{}))
}
