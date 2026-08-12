package ack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFileTotals(t *testing.T) {
	// Expected values taken from the first COUNT section in each raw file
	// (file-level summary, located via the -----COUNT-----AMOUNT----- header).
	cases := []struct {
		name string
		file string
		want FileTotals
	}{
		{
			name: "production file with 1 batch",
			file: "ACHFAHK673960043AIN202605261654134.ack",
			want: FileTotals{Batches: 1, Entries: 1, DebitTotal: 0, CreditTotal: 32145},
		},
		{
			name: "production file pended with file-level errors",
			file: "ACHFAHK673960043AIN202608121530761.ack",
			want: FileTotals{Batches: 1, Entries: 2, DebitTotal: 101, CreditTotal: 101},
		},
		{
			name: "production file accepted with no errors",
			file: "ACHFAHK673960043AIN202608121534803.ack",
			want: FileTotals{Batches: 1, Entries: 2, DebitTotal: 101, CreditTotal: 101},
		},
		{
			name: "production file with 1 batch (variant)",
			file: "ACHFAHK673960043AIN202605281447969.ack",
			want: FileTotals{Batches: 1, Entries: 1, DebitTotal: 0, CreditTotal: 32145},
		},
		{
			name: "production accepted file 5 batches",
			file: "achfahk691000134ain20200512085211052.ack",
			want: FileTotals{Batches: 5, Entries: 54, DebitTotal: 16839098, CreditTotal: 3938524},
		},
		{
			name: "production file with file-level errors",
			file: "achfahk691000134ain20200512085211959.ack",
			want: FileTotals{Batches: 6, Entries: 54, DebitTotal: 16080598, CreditTotal: 2940024},
		},
		{
			name: "Fed doc example - file-level errors page",
			file: "file-level-example.ack",
			want: FileTotals{Batches: 10081, Entries: 10384, DebitTotal: 739193503, CreditTotal: 345563718},
		},
		{
			name: "Fed doc example - multi-batch",
			file: "file-multi-batch-level-example.ack",
			want: FileTotals{Batches: 4, Entries: 396, DebitTotal: 3296034, CreditTotal: 8003806},
		},
	}

	rawDir := filepath.Join("..", "..", "testdata", "ack", "raw")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(rawDir, tc.file)
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			recs := Split(data)
			require.NotEmpty(t, recs)

			got, err := ParseFileTotals(recs)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseFileTotals_Empty(t *testing.T) {
	got, err := ParseFileTotals(nil)
	require.NoError(t, err)
	require.Equal(t, FileTotals{}, got)

	got, err = ParseFileTotals([]Record{})
	require.NoError(t, err)
	require.Equal(t, FileTotals{}, got)
}
