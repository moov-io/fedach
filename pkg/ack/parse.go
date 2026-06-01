package ack

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// countHeaderRe matches the distinctive dashed header that starts the first
// (file-level) COUNT/AMOUNT section.
var countHeaderRe = regexp.MustCompile(`-+\s*COUNT\s*-+\s*AMOUNT\s*-+`)

// batchesRe extracts the batch count and debit total from the BATCHES line
// inside the first COUNT section.
var batchesRe = regexp.MustCompile(`BATCHES:\s*([\d,]+)\s+DEBIT:\s*\$?\s*([\d,]+\.\d{2})`)

// entriesRe extracts the entry/addenda count and credit total from the
// ENTRY/ADDENDA line inside the first COUNT section.
var entriesRe = regexp.MustCompile(`ENTRY/ADDENDA:\s*([\d,]+)\s+CREDITS:\s*\$?\s*([\d,]+\.\d{2})`)

type FileTotals struct {
	Batches int
	Entries int

	DebitTotal  int64
	CreditTotal int64
}

// ParseFileTotals extracts the file-level totals from the first COUNT section
// in a parsed ACK file. The section is located by its distinctive header
// containing "COUNT" and "AMOUNT" surrounded by dashes (e.g.
// "---------------------COUNT-----------------AMOUNT--------").
//
// It returns Batches and Entries counts (as printed), and the Debit/Credit
// totals in integer cents (pennies). The first COUNT section is always the
// file-level summary (later per-batch COUNT sections use different tags like R/S).
func ParseFileTotals(recs []Record) (FileTotals, error) {
	if len(recs) == 0 {
		return FileTotals{}, nil
	}

	// Build full text preserving order. Use \n as safe separator so patterns
	// don't accidentally glue across record boundaries.
	var b strings.Builder
	for _, r := range recs {
		b.Write(r.Content)
		b.WriteByte('\n')
	}
	full := b.String()

	// Locate the first COUNT header using the dashes the user specified.
	// This skips any earlier non-count sections.
	loc := countHeaderRe.FindStringIndex(full)
	searchFrom := 0
	if loc != nil {
		searchFrom = loc[1]
	} else if strings.Contains(full, "COUNT") && strings.Contains(full, "BATCHES") {
		// Fallback for odd layouts: start from the first COUNT mention.
		if idx := strings.Index(full, "COUNT"); idx >= 0 {
			searchFrom = idx
		}
	} else {
		return FileTotals{}, fmt.Errorf("no COUNT section found")
	}

	tail := full[searchFrom:]

	bm := batchesRe.FindStringSubmatch(tail)
	if bm == nil {
		return FileTotals{}, fmt.Errorf("BATCHES line not found in first COUNT section")
	}
	em := entriesRe.FindStringSubmatch(tail)
	if em == nil {
		return FileTotals{}, fmt.Errorf("ENTRY/ADDENDA line not found in first COUNT section")
	}

	batches, err := parseCount(bm[1])
	if err != nil {
		return FileTotals{}, fmt.Errorf("parsing BATCHES count: %w", err)
	}
	entries, err := parseCount(em[1])
	if err != nil {
		return FileTotals{}, fmt.Errorf("parsing ENTRY/ADDENDA count: %w", err)
	}
	debit, err := parseAmount(bm[2])
	if err != nil {
		return FileTotals{}, fmt.Errorf("parsing DEBIT total: %w", err)
	}
	credit, err := parseAmount(em[2])
	if err != nil {
		return FileTotals{}, fmt.Errorf("parsing CREDITS total: %w", err)
	}

	return FileTotals{
		Batches:     batches,
		Entries:     entries,
		DebitTotal:  debit,
		CreditTotal: credit,
	}, nil
}

func parseCount(s string) (int, error) {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func parseAmount(s string) (int64, error) {
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSpace(s)
	if s == "" || s == "0.00" || s == "0" {
		return 0, nil
	}
	// Expect "123456.78" or "12345678" (no decimal = whole dollars)
	if idx := strings.Index(s, "."); idx != -1 {
		dolStr := s[:idx]
		cenStr := s[idx+1:]
		if len(cenStr) == 1 {
			cenStr += "0"
		} else if len(cenStr) > 2 {
			cenStr = cenStr[:2]
		}
		dol, _ := strconv.Atoi(dolStr)
		cen, _ := strconv.Atoi(cenStr)
		return int64(dol)*100 + int64(cen), nil
	}
	dol, _ := strconv.Atoi(s)
	return int64(dol) * 100, nil
}
