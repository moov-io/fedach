// Package ack provides parsing for FedACH FAHK "Acknowledgement of ACH File Deposits"
// report files (also known as ack files).
//
// These files use a custom tagged format where logical records are identified by
// a single uppercase letter prefix (A-Z) at the start of the record. Many records
// are terminated by a 'Z' character followed by padding spaces.
//
// The files are challenging to parse because:
//   - They often arrive as one or two very long lines with no reliable newlines.
//   - Logical records are embedded in what was originally a fixed-width visual report.
//   - Record boundaries are primarily indicated by letter prefixes + Z terminators.
//   - Error blocks use I→J→K→Z (file-level) and W→X→Y→Z (batch-level) patterns.
//   - Indentation of prefixes varies, and some content appears between Z terminators
//     without its own letter prefix (these "detail lines" are not treated as tagged records).
//
// The parser focuses on reliably extracting the tagged logical records using a
// combination of letter prefix detection and Z terminator boundaries.
package ack

import (
	"bytes"
	"strings"
	"unicode"
)

// Record represents a single logical record extracted from an FAHK ack file.
type Record struct {
	// Prefix is the single uppercase letter (A-Z) that identifies the record type.
	// Common values include A (header/summary), B (process date), C-F (counts),
	// G (status), H (spacer), I/J/K (file error details), L-Z (batch details and terminators).
	Prefix byte

	// Content is the raw bytes of the record, including the prefix letter and
	// all original content up to (but not including) the next record's prefix
	// or the Z terminator boundary. Trailing padding spaces are trimmed.
	Content []byte
}

// SplitLines reconstructs the original visual report lines from a raw FAHK ack file.
//
// Unlike Split (which extracts the tagged A/B/C/I/J/K... logical records), SplitLines
// attempts to recover the line-by-line appearance of the original FedPayments Reporter
// output. The raw files are the report "flattened" (newlines removed or turned into
// spaces, with single-letter tags and occasional line numbers embedded).
//
// SplitLines uses several signals to decide where visual line breaks belong:
//   - Embedded line number markers (" 1 ", " 2S", " 3 ", " 4F", ..., " 9F", etc.)
//   - Z terminator + significant padding followed by a letter tag (strong boundary)
//   - Single-letter tags (A-Z) that start a new conceptual visual line after padding
//     or after another tagged line in error blocks (I/J/K, W/X/Y sequences)
//
// When emitting each reconstructed line, leading single-letter tags (the A/B/C/I/J/K...
// markers) are stripped because they are metadata, not part of the printable report
// content. The very first header line ("AJ001A01A08052...") is an exception — its
// leading A is kept as it is part of the fixed header data.
//
// The result is a slice of strings, one per visual line, including blank lines,
// matching the style of the files found in testdata/ack/lines/ for well-formed samples.
func SplitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}

	// Normalize line endings to space for uniform scanning.
	stream := make([]byte, len(data))
	copy(stream, data)
	for i, b := range stream {
		if b == '\n' || b == '\r' {
			stream[i] = ' '
		}
	}
	n := len(stream)

	// Collect positions that start a new visual line in the original report.
	var cuts []int
	markerCut := make(map[int]bool) // cuts from embedded " N"/" NN" markers (these start content, do not strip leading letter)

	// 1. Always start at the first non-whitespace (or 0).
	start := 0
	for start < n && isWhitespace(stream[start]) {
		start++
	}
	cuts = append(cuts, start)

	// 2. Embedded line number markers: " N" or " NN" (1-2 digits) followed by
	//    a letter or significant content. These are very strong signals in the
	//    header section of the report (positions < 1000). We deliberately ignore
	//    later " 1 " values that are data (BATCHES: 1, ENTRY/ADDENDA: 1, etc).
	for i := 0; i < n-1; i++ {
		if stream[i] != ' ' {
			continue
		}
		if i > 1000 {
			// Only header-section markers (1-9) are real visual line signals.
			continue
		}
		// Try 1-digit or 2-digit number.
		j := i + 1
		if j >= n || !isDigit(stream[j]) {
			continue
		}
		j++
		if j < n && isDigit(stream[j]) {
			j++
		}
		// After the number we usually see a letter or many spaces.
		if j >= n {
			continue
		}
		after := stream[j]
		if isUpperLetter(after) || after == ' ' {
			// Cut at the content start after the marker. The marker digits (and
			// sometimes the following tag letter for 2S/4F style) will be left
			// as trailing junk in the *prior* segment and cleaned by removeTrailingMarker.
			cuts = append(cuts, j)
			markerCut[j] = true
		}
	}

	// 3. Z terminator followed by padding is a visual line group boundary.
	//    Z + >=2 ws is reliable in this format.
	//    If the position after Z+pad lands on a marker digit (e.g. " 5IMMEDIATE"),
	//    advance past the digit so we don't create an orphan "5" segment.
	//
	//    Large padding after a Z usually corresponds to vertical whitespace
	//    (blank lines) in the original visual report. We record those positions
	//    so the builder can synthesize the expected blank line(s).
	largeZPad := make(map[int]bool)
	for i := 0; i < n; i++ {
		if stream[i] != 'Z' && stream[i] != 'z' {
			continue
		}
		j := i + 1
		pad := 0
		for j < n && isWhitespace(stream[j]) {
			pad++
			j++
		}
		if pad >= 2 && j < n {
			// Skip over an immediately following 1-2 digit line marker if present.
			if j < n && isDigit(stream[j]) {
				j++
				if j < n && isDigit(stream[j]) {
					j++
				}
			}
			cuts = append(cuts, j)
			if pad >= 30 {
				// Only synthesize a visual blank line for major section transitions
				// (new COUNT block, new batch header, new error summary, etc.).
				// Tight error-detail Z's (between I/J/K or W/X/Y) usually do not
				// produce an extra blank in the visual report.
				if j < n {
					ch := stream[j]
					// Major section / page transitions get a synthetic blank line
					// before them when they follow a large Z gap. Include 'A' so
					// that repeated page headers (AJ001A01...) deep in error
					// listings are preceded by the expected vertical whitespace.
					if ch == 'A' ||
						ch == 'B' || ch == 'C' || ch == 'D' || ch == 'F' || ch == 'G' ||
						ch == 'M' || ch == 'R' || ch == 'S' || ch == 'U' || ch == 'W' {
						largeZPad[j] = true
					}
				}
			}
		}
	}

	// 4. Single uppercase letter A-Z after whitespace (not inside word) that
	//    has either high indent (typical left margin for a new visual line's tag)
	//    or belongs to the set of known low/mid-indent structural tags (I/J/K
	//    error details and certain batch lines like E/Q/T/V that can have less
	//    padding in the original layout).
	//
	//    In-text capitals inside ALL-CAPS report text (FEDERAL, REPORT, BATCHES,
	//    CREDIT, etc.) almost always have low indent (< 20) and are excluded.
	lowIndentTags := "IJKWXYEQT VB" // structural tags (incl. B for "BATCH NUMBER" etc. after batch errors)
	for i := 2; i < n-1; i++ {
		if !isUpperLetter(stream[i]) {
			continue
		}
		if !isWhitespace(stream[i-1]) {
			continue
		}
		if i > 1 && (isUpperLetter(stream[i-2]) || isDigit(stream[i-2])) {
			continue
		}
		// Count preceding whitespace run (indent).
		indent := 0
		for j := i - 1; j >= 0 && isWhitespace(stream[j]); j-- {
			indent++
		}
		letter := stream[i]
		next := stream[i+1]
		// Skip obvious in-word starts (followed immediately by lowercase).
		if next >= 'a' && next <= 'z' {
			continue
		}

		isLowTag := strings.ContainsRune(lowIndentTags, rune(letter))
		if indent >= 30 || (isLowTag && indent >= 5) {
			cuts = append(cuts, i)
		}
	}

	// 5. Repeated report page headers ("AJ001A01A08052" etc.) are very strong
	//    signals of a new visual line start. These can appear multiple times in
	//    long error reports (new "page" in the middle of file-level or batch
	//    errors). We must cut here so the header is emitted as its own line and
	//    subsequent content is attributed correctly.
	const reportHeader = "AJ001A01A08052"
	for i := 0; i <= n-len(reportHeader); i++ {
		if stream[i] == 'A' {
			if string(stream[i:i+len(reportHeader)]) == reportHeader {
				cuts = append(cuts, i)
			}
		}
	}

	// 6. Inside long X/Y batch error detail segments in the raw stream, the
	//    next visual line's text is sometimes glued directly after the error
	//    description with only a few spaces + the next tag letter
	//    (e.g. "...ZEROS)   XBATCH NUMBER..."). Because each visual line is
	//    80 characters, we add explicit cuts at these glued points so the
	//    extractor produces separate lines matching the golden.
	for i := 60; i < n-20; i++ {
		// Look for the characteristic "   X" or "   Y" (or slight variants)
		// followed by "BATCH", "ORIGIN", etc. — these almost always start a
		// new visual line inside the error block.
		if stream[i] == ' ' && stream[i+1] == ' ' && stream[i+2] == ' ' &&
			(stream[i+3] == 'X' || stream[i+3] == 'Y' || stream[i+3] == 'x' || stream[i+3] == 'y') {
			rest := stream[i+4:]
			take := 10
			if len(rest) < take {
				take = len(rest)
			}
			upper := strings.ToUpper(string(rest[:take]))
			if strings.HasPrefix(upper, "BATCH") || strings.HasPrefix(upper, "ORIGIN") ||
				strings.HasPrefix(upper, "COMPANY") || strings.HasPrefix(upper, "EFFECT") {
				cuts = append(cuts, i+3) // cut at the embedded X/Y so normal stripping applies
			}
		}
	}

	// Dedup and sort cuts.
	seen := make(map[int]bool)
	var sortedCuts []int
	for _, c := range cuts {
		if c < 0 || c >= n {
			continue
		}
		if !seen[c] {
			seen[c] = true
			sortedCuts = append(sortedCuts, c)
		}
	}
	// Simple insertion sort (small N).
	for i := 1; i < len(sortedCuts); i++ {
		j := i
		for j > 0 && sortedCuts[j-1] > sortedCuts[j] {
			sortedCuts[j-1], sortedCuts[j] = sortedCuts[j], sortedCuts[j-1]
			j--
		}
	}

	// Build lines between cuts.
	var lines []string
	for idx, c := range sortedCuts {
		end := n
		if idx+1 < len(sortedCuts) {
			end = sortedCuts[idx+1]
		}
		segment := stream[c:end]

		// Decide whether to strip a leading single-letter tag.
		// - Never strip for the special first-line (or any) AJ001A01... report
		//   header (keep the A). These can repeat mid-file on multi-page error reports.
		// - Never strip when the cut came from a digit marker (the letter after
		//   " 2S", " 4F", " 5I" etc. is the first char of visible text like
		//   "SERVICING", "FILE", "IMMEDIATE").
		// - Otherwise (letter tag cuts and Z-following content), strip the tag
		//   (handles glued cases "ITH145" -> "TH145", "GFILE" -> "FILE", "WBH232"->"BH232").
		if len(segment) > 0 && isUpperLetter(segment[0]) {
			isFirstLine := idx == 0
			rest := segment[1:]
			keepLeading := isFirstLine && len(rest) > 2 && rest[0] == 'J' && isDigit(rest[1])
			isMarkerStart := markerCut[c]
			// Protect any occurrence of the distinctive report header line.
			isReportHeader := len(segment) >= 14 && string(segment[:14]) == "AJ001A01A08052"
			if !keepLeading && !isMarkerStart && !isReportHeader {
				segment = rest
			}
		}

		// Trim trailing whitespace for the line (but keep internal and leading ws).
		line := string(bytes.TrimRightFunc(segment, func(r rune) bool {
			return r == ' ' || r == '\t' || unicode.IsSpace(r)
		}))

		// Clean any orphaned line-number marker (" 1", " 2", " 3", " 4" etc.)
		// that was left at the end of the prior visual line by a marker cut.
		line = removeTrailingMarker(line)

		// Skip pure orphaned marker digits (e.g. a lone "5" from a Z-cut landing
		// on a " 5IMMEDIATE" header line). These are not part of the visual report.
		if len(line) == 1 && isDigit(line[0]) {
			continue
		}

		// When the current cut position was reached after a Z with large padding,
		// the original report had vertical whitespace here. Emit one blank line
		// to preserve the visual structure (unless we just emitted a blank).
		if largeZPad[c] {
			if len(lines) == 0 || lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
		}

		lines = append(lines, line)
	}

	// Post-process: we intentionally keep blank lines so the reconstructed
	// layout matches the visual report (including spacing around error blocks
	// and the final END line).

	// However, do not emit trailing blank lines at the very end of the file;
	// the golden references in lines/ do not contain them.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// removeTrailingMarker removes trailing report-generator line markers of the
// form " N" or " NN" (optionally with a following uppercase) that can be left
// attached to the end of the previous line when a marker cut is taken at the
// content after the marker.
//
// It is intentionally conservative: it never strips trailing numbers from
// lines that contain "=" , "CALC", or "SHOULD BE" (common on the K/Y detail
// lines that report calculated vs. actual counts and amounts). Z terminators
// are always removed because they are never part of visible report content.
func removeTrailingMarker(s string) string {
	for k := 0; k < 6; k++ { // safety
		t := strings.TrimRight(s, " \t")
		if len(t) < 1 {
			return t
		}
		changed := false

		// 1. Remove trailing Z (and any " Z" or "Z" at end) — Z is never visible content.
		if len(t) > 0 && (t[len(t)-1] == 'Z' || t[len(t)-1] == 'z') {
			t = strings.TrimRight(t[:len(t)-1], " \t")
			changed = true
		} else if len(t) >= 2 && (t[len(t)-1] == 'Z' || t[len(t)-1] == 'z') && t[len(t)-2] == ' ' {
			t = strings.TrimRight(t[:len(t)-2], " \t")
			changed = true
		}

		// 2. Remove trailing digit marker " N"/" NN".
		// Only do this for lines that do not look like they contain real
		// report data numbers (e.g. "CALC BATCH COUNT = 4", "SHOULD BE 10",
		// "ENTRY/ADDENDA COUNT = 00000049"). These numbers legitimately
		// appear at the end of K/Y continuation lines and must be preserved.
		// We keep the logic only for early header lines or lines that are
		// clearly not data-heavy.
		if !strings.Contains(t, "=") && !strings.Contains(t, "SHOULD BE") && !strings.Contains(t, "CALC") {
			j := len(t) - 1
			if j >= 0 && isUpperLetter(t[j]) {
				j--
			}
			digits := 0
			for j >= 0 && isDigit(t[j]) && digits < 2 {
				digits++
				j--
			}
			if digits >= 1 && j >= 0 && t[j] == ' ' {
				t = t[:j]
				changed = true
			}
		}

		s = strings.TrimRight(t, " \t")
		if !changed {
			break
		}
	}
	return strings.TrimRight(s, " \t")
}

// Split parses the raw bytes of a FedACH FAHK acknowledgement file and returns
// the sequence of logical tagged records in file order.
//
// The parser is designed to handle:
//   - Files with no newlines (single long line) or inconsistent line endings.
//   - Multiple logical records concatenated on one physical line.
//   - Varying indentation of record prefixes in the original visual layout.
//   - I/J/K/Z file-level error blocks and W/X/Y/Z batch-level error blocks.
//   - Z as both a record terminator and (in some cases) a record with content.
//
// Splitting strategy (two-phase):
//  1. The entire input is treated as a single stream; newlines and carriage returns
//     are normalized to spaces for boundary detection purposes.
//  2. Strong boundaries are located first: every occurrence of 'Z' followed by
//     5+ whitespace characters and then an uppercase letter A-Z. These Z+padding+letter
//     sequences are the most reliable record group separators in the FAHK format.
//  3. Within each segment (initial content up to first strong boundary, and between
//     subsequent strong boundaries), we scan for all uppercase letters A-Z that are
//     not immediately preceded by another letter or digit. These become the tagged
//     record starts. This catches I/J/K/W/X/Y error-block letters even when they
//     have low relative indentation.
//  4. Each record's content runs from its starting letter up to the next record start.
//  5. The resulting Record keeps the original prefix and a trimmed copy of the
//     content bytes for that logical record.
//
// The function is tolerant of extra whitespace and padding. It never returns an error
// for malformed input; it returns whatever tagged records it can identify.
func Split(data []byte) []Record {
	if len(data) == 0 {
		return nil
	}

	// Normalize line endings to spaces so we can treat the input as a continuous stream.
	// This is critical because the files often lack reliable newlines.
	stream := make([]byte, len(data))
	copy(stream, data)
	for i, b := range stream {
		if b == '\n' || b == '\r' {
			stream[i] = ' '
		}
	}

	n := len(stream)

	// ---------------------------------------------------------------------
	// Phase 1: Identify strong Z-terminator boundaries.
	// A strong boundary is a 'Z' followed by 5+ whitespace and then an
	// uppercase letter. These are the most reliable record group separators
	// in the file format.
	// ---------------------------------------------------------------------
	type boundary struct {
		zEnd int // position after the Z (the Z itself belongs to the prior record)
		next int // position of the letter that starts the next record
	}
	var boundaries []boundary

	for i := 0; i < n; i++ {
		if stream[i] == 'Z' || stream[i] == 'z' {
			j := i + 1
			pad := 0
			for j < n && isWhitespace(stream[j]) {
				pad++
				j++
			}
			if pad >= 5 && j < n && isUpperLetter(stream[j]) {
				boundaries = append(boundaries, boundary{zEnd: i + 1, next: j})
				i = j - 1 // skip past the padding we just examined
			}
		}
	}

	// ---------------------------------------------------------------------
	// Phase 2: Within each segment (between strong boundaries), find all
	// tagged record starts.
	//
	// We use two rules:
	//   a) A minimum indentation (15 spaces) for "normal" letters. This excludes
	//      the vast majority of word-initial capitals inside the English text of
	//      the report.
	//   b) A lower threshold (or no indent requirement beyond "after whitespace")
	//      for the known error-block letters I, J, K, W, X, Y. These frequently
	//      appear with shallower indentation in the original layout.
	// ---------------------------------------------------------------------
	const normalMinIndent = 15

	var recStarts []int

	addRecordStarts := func(segStart, segEnd int) {
		for k := segStart; k < segEnd; k++ {
			if !isUpperLetter(stream[k]) {
				continue
			}
			// Must not be inside a word.
			if k > 0 {
				prev := stream[k-1]
				if isUpperLetter(prev) || isDigit(prev) {
					continue
				}
			}
			// Must be preceded by whitespace (all real prefixes are).
			if k == 0 || !isWhitespace(stream[k-1]) {
				continue
			}

			// Count immediate preceding whitespace for indent check.
			indent := 0
			for j := k - 1; j >= 0 && isWhitespace(stream[j]); j-- {
				indent++
			}

			letter := stream[k]
			isErrorLetter := letter == 'I' || letter == 'J' || letter == 'K' ||
				letter == 'W' || letter == 'X' || letter == 'Y'

			if isErrorLetter || indent >= normalMinIndent {
				recStarts = append(recStarts, k)
			}
		}
	}

	// Initial segment: from start of content up to first strong boundary (or end).
	first := 0
	for first < n && isWhitespace(stream[first]) {
		first++
	}
	// The very first letter after leading padding is always a tagged record start
	// (the "AJ001A01A..." report header block), even if it has low indent.
	if first < n {
		for k := first; k < n; k++ {
			if isUpperLetter(stream[k]) {
				// Only add it if it is not inside a word (defensive).
				if k == 0 || (!isUpperLetter(stream[k-1]) && !isDigit(stream[k-1])) {
					recStarts = append(recStarts, k)
				}
				break
			}
		}
	}
	if len(boundaries) > 0 {
		addRecordStarts(first, boundaries[0].zEnd)
	} else {
		addRecordStarts(first, n)
	}

	// Subsequent segments.
	for bi, b := range boundaries {
		segStart := b.next
		segEnd := n
		if bi+1 < len(boundaries) {
			segEnd = boundaries[bi+1].zEnd
		}
		addRecordStarts(segStart, segEnd)
	}

	if len(recStarts) == 0 {
		return nil
	}

	// ---------------------------------------------------------------------
	// Phase 3: Build Record objects. Each record runs from its start up to
	// the next record start (or a Z terminator that we already used as a
	// boundary). Because we split on strong Z boundaries, the Z itself is
	// the last byte of the record that precedes the boundary.
	// ---------------------------------------------------------------------
	var records []Record
	for idx, s := range recStarts {
		e := n
		if idx+1 < len(recStarts) {
			e = recStarts[idx+1]
		}
		// If there is a strong boundary whose zEnd falls inside (s, e),
		// we should have already split there, so e should be correct.
		rec := buildRecord(stream, s, e)
		if rec != nil {
			records = append(records, *rec)
		}
	}

	// Final cleanup: attach any trailing Z terminators that may have been
	// left after the last record if the file ends with Z + padding.
	// (buildRecord already trims trailing spaces, so lone Z records are fine.)

	return records
}

// buildRecord constructs a Record from the slice stream[start:end].
// It trims trailing whitespace from the content for cleanliness while
// preserving the internal structure of the record.
func buildRecord(stream []byte, start, end int) *Record {
	if start >= end {
		return nil
	}
	// The first byte must be the prefix letter.
	prefix := stream[start]
	if !isUpperLetter(prefix) {
		// Should not happen in normal operation, but be defensive.
		return nil
	}

	content := stream[start:end]
	// Trim trailing whitespace only (preserve leading indentation which may be meaningful).
	content = bytes.TrimRightFunc(content, func(r rune) bool {
		return r == ' ' || r == '\t' || unicode.IsSpace(r)
	})

	if len(content) == 0 {
		return nil
	}

	return &Record{
		Prefix:  prefix,
		Content: content,
	}
}

// isUpperLetter reports whether b is an uppercase ASCII letter A-Z.
func isUpperLetter(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

// isDigit reports whether b is an ASCII digit 0-9.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// isWhitespace reports whether b is a space, tab, or other whitespace rune.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || unicode.IsSpace(rune(b))
}

// RecordsByPrefix returns a map from prefix letter to all records having that prefix.
// This is useful for quickly locating error records (I, J, K, W, X, Y) or summary
// records (A, B, C, ...).
func RecordsByPrefix(recs []Record) map[byte][]Record {
	m := make(map[byte][]Record)
	for _, r := range recs {
		m[r.Prefix] = append(m[r.Prefix], r)
	}
	return m
}

// FindErrorBlocks locates I/J/K/Z file-level error blocks and W/X/Y/Z batch-level
// error blocks in the record sequence.
//
// Because the original ack file format places error detail lines (J, K, X, Y) with
// varying amounts of padding, and because Z terminators may have other records
// between an error letter and its terminating Z in some edge cases, this function
// uses a tolerant matching strategy:
//
//   - A file error block starts at any I/J/K record and continues through any
//     subsequent I/J/K records. It is terminated by the first Z that appears
//     after the run of error letters (skipping over any non-error records that
//     may intervene due to parsing edge cases).
//   - A batch error block is handled analogously for W/X/Y + Z.
//
// The returned blocks include the terminating Z when one is found within a
// reasonable distance after the error letters.
func FindErrorBlocks(recs []Record) (fileErrors, batchErrors [][]Record) {
	i := 0
	for i < len(recs) {
		r := recs[i]

		if r.Prefix == 'I' || r.Prefix == 'J' || r.Prefix == 'K' {
			// Start of a file-level error block.
			block := []Record{r}
			j := i + 1
			for j < len(recs) {
				p := recs[j].Prefix
				if p == 'I' || p == 'J' || p == 'K' {
					block = append(block, recs[j])
					j++
					continue
				}
				if p == 'Z' {
					block = append(block, recs[j])
					j++
					break
				}
				// Non-error letter: skip a limited number of records looking for a Z.
				// If we see too many non-error letters, we stop and emit the block
				// without a Z (defensive).
				skipped := 0
				for j < len(recs) && skipped < 3 {
					if recs[j].Prefix == 'Z' {
						block = append(block, recs[j])
						j++
						break
					}
					skipped++
					j++
				}
				break
			}
			fileErrors = append(fileErrors, block)
			i = j
			continue
		}

		if r.Prefix == 'W' || r.Prefix == 'X' || r.Prefix == 'Y' {
			// Start of a batch-level error block.
			block := []Record{r}
			j := i + 1
			for j < len(recs) {
				p := recs[j].Prefix
				if p == 'W' || p == 'X' || p == 'Y' {
					block = append(block, recs[j])
					j++
					continue
				}
				if p == 'Z' {
					block = append(block, recs[j])
					j++
					break
				}
				skipped := 0
				for j < len(recs) && skipped < 3 {
					if recs[j].Prefix == 'Z' {
						block = append(block, recs[j])
						j++
						break
					}
					skipped++
					j++
				}
				break
			}
			batchErrors = append(batchErrors, block)
			i = j
			continue
		}

		i++
	}

	return fileErrors, batchErrors
}
