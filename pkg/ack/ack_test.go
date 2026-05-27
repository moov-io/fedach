package ack

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplit_BasicStructure(t *testing.T) {
	// Load one of the raw ack files that has file-level errors (I/J/K/Z blocks).
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ack", "raw", "achfahk691000134ain20200512085211959.ack"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	recs := Split(data)
	if len(recs) == 0 {
		t.Fatal("Split returned no records")
	}

	// We expect a reasonable number of tagged records (the file has 93 in our analysis).
	if len(recs) < 30 {
		t.Errorf("expected at least 30 records, got %d", len(recs))
	}

	// The first record should start with 'A' (the report header).
	if recs[0].Prefix != 'A' {
		t.Errorf("first record prefix = %c, want A", recs[0].Prefix)
	}

	// Verify that every record has a valid prefix A-Z.
	for i, r := range recs {
		if r.Prefix < 'A' || r.Prefix > 'Z' {
			t.Errorf("record[%d] has invalid prefix %c (0x%02x)", i, r.Prefix, r.Prefix)
		}
		if len(r.Content) == 0 {
			t.Errorf("record[%d] has empty content", i)
		}
		if r.Content[0] != r.Prefix {
			t.Errorf("record[%d] content does not start with its prefix", i)
		}
	}
}

func TestSplit_ErrorBlocks(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ack", "raw", "achfahk691000134ain20200512085211959.ack"))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	recs := Split(data)
	fileErrs, batchErrs := FindErrorBlocks(recs)

	// This file has multiple file-level errors (IFC104, IFH004, etc.).
	if len(fileErrs) == 0 {
		t.Error("expected to find at least one I/J/K/Z file error block")
	}

	// Verify each file error block contains at least one I/J/K record.
	for i, block := range fileErrs {
		if len(block) == 0 {
			t.Errorf("fileErrs[%d] is empty", i)
			continue
		}
		first := block[0].Prefix
		if first != 'I' && first != 'J' && first != 'K' {
			t.Errorf("fileErrs[%d] starts with %c, want I/J/K", i, first)
		}
		// The block should end with Z if a terminator was found nearby.
		// We do not hard-require it because some error blocks in the wild may
		// be truncated; the tolerant matcher in FindErrorBlocks will still return
		// the I/J/K letters.
	}

	// This file also has batch-level errors (W/X/Y/Z).
	if len(batchErrs) == 0 {
		t.Error("expected to find at least one W/X/Y/Z batch error block")
	}

	for i, block := range batchErrs {
		if len(block) == 0 {
			t.Errorf("batchErrs[%d] is empty", i)
			continue
		}
		first := block[0].Prefix
		if first != 'W' && first != 'X' && first != 'Y' {
			t.Errorf("batchErrs[%d] starts with %c, want W/X/Y", i, first)
		}
	}
}

func TestSplit_AllRawFiles(t *testing.T) {
	rawDir := filepath.Join("..", "..", "testdata", "ack", "raw")
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		t.Fatalf("failed to read raw dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".ack" {
			continue
		}
		path := filepath.Join(rawDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read %s: %v", e.Name(), err)
			continue
		}

		recs := Split(data)
		if len(recs) == 0 {
			t.Errorf("%s: Split returned zero records", e.Name())
			continue
		}

		// Every record must start with its own prefix byte.
		for i, r := range recs {
			if len(r.Content) == 0 || r.Content[0] != r.Prefix {
				t.Errorf("%s: record[%d] content mismatch (prefix=%c)", e.Name(), i, r.Prefix)
			}
		}
	}
}

func TestSplit_EmptyAndTrivial(t *testing.T) {
	if recs := Split(nil); len(recs) != 0 {
		t.Errorf("Split(nil) = %d records, want 0", len(recs))
	}
	if recs := Split([]byte{}); len(recs) != 0 {
		t.Errorf("Split([]byte{}) = %d records, want 0", len(recs))
	}
	if recs := Split([]byte("     ")); len(recs) != 0 {
		t.Errorf("Split(whitespace only) = %d records, want 0", len(recs))
	}
}

func TestSplit_PreservesContent(t *testing.T) {
	// Use a small, well-known snippet that contains an I/J/K/Z sequence.
	input := []byte("                                        IFC104-INCOMPLETE BLOCKING ON FILE                                               J                                                                                K                                                                                Z                                                  ")

	recs := Split(input)
	if len(recs) < 4 {
		t.Fatalf("expected at least 4 records (I,J,K,Z), got %d", len(recs))
	}

	// Find the I record.
	var foundI bool
	for _, r := range recs {
		if r.Prefix == 'I' && bytes.Contains(r.Content, []byte("IFC104")) {
			foundI = true
			if !bytes.HasPrefix(r.Content, []byte("I")) {
				t.Error("I record content should start with 'I'")
			}
		}
	}
	if !foundI {
		t.Error("did not find I record containing IFC104")
	}
}

func TestSplitLines_GoldenFiles(t *testing.T) {
	// Table-driven golden tests for the line extractor (SplitLines).
	// Raw inputs live in testdata/ack/raw/, the corresponding expected visual
	// lines live in testdata/ack/lines/ under the exact same basename.
	// This makes it trivial to add more cases: just drop a new pair of files.
	cases := []string{
		// Only list basenames here when a verified golden exists in lines/ for that raw file.
		// This keeps the table test trivial ("just a filename") while guaranteeing that
		// raw/ and lines/ subdirectories stay cleanly separated.
		"ACHFAHK673960043AIN202605261654134.ack",
		"achfahk691000134ain20200512085211052.ack",
		"achfahk691000134ain20200512085211959.ack",
	}

	rawDir := filepath.Join("..", "..", "testdata", "ack", "raw")
	linesDir := filepath.Join("..", "..", "testdata", "ack", "lines")

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			rawPath := filepath.Join(rawDir, name)
			goldPath := filepath.Join(linesDir, name)

			raw, err := os.ReadFile(rawPath)
			if err != nil {
				t.Fatalf("failed to read raw: %v", err)
			}
			goldBytes, err := os.ReadFile(goldPath)
			if err != nil {
				t.Fatalf("failed to read gold: %v", err)
			}

			goldLines := strings.Split(strings.TrimRight(string(goldBytes), "\n"), "\n")
			got := SplitLines(raw)

			if len(got) != len(goldLines) {
				t.Fatalf("SplitLines produced %d lines, gold has %d", len(got), len(goldLines))
			}
			for i := range goldLines {
				if got[i] != goldLines[i] {
					t.Errorf("line %d mismatch\n  got:  %q\n  want: %q", i+1, got[i], goldLines[i])
				}
			}
		})
	}
}
