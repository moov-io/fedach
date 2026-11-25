package ack_test

import (
	"fmt"
	"log"
	"os"

	"github.com/moov-io/fedach/pkg/ack"
)

func Example() {
	data, err := os.ReadFile("../../testdata/ack/raw/ACHFAHK673960043AIN202605261654134.ack")
	if err != nil {
		log.Fatal(err)
	}

	recs := ack.Split(data)
	fmt.Printf("records: %d\n", len(recs))

	fileErrs, batchErrs := ack.FindErrorBlocks(recs)
	fmt.Printf("file errors: %d\n", len(fileErrs))
	fmt.Printf("batch errors: %d\n", len(batchErrs))
	fmt.Println(ack.FormatErrorBlock(fileErrs[0]))

	byPrefix := ack.RecordsByPrefix(recs)
	fmt.Printf("I records: %d\n", len(byPrefix['I']))

	totals, err := ack.ParseFileTotals(recs)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("batches: %d debit cents: %d credit cents: %d\n",
		totals.Batches, totals.DebitTotal, totals.CreditTotal)

	// Output:
	// records: 45
	// file errors: 2
	// batch errors: 1
	// TH145-SENDING ELECTRONIC CONNECTION OWNER NOT AUTHORIZED TO SEND ACH FILE SENDING ELECTRONIC CONNECTION OWNER = SECO ID = G-C005C5
	// I records: 2
	// batches: 1 debit cents: 0 credit cents: 32145
}
