package db

import (
	"testing"
	"time"

	"github.com/Abhinav7903/Bitcoin-Indexer/internal/models"
)

func TestAggregateAddressTransactionsSumsDuplicateAddressTxRows(t *testing.T) {
	blockTime := time.Unix(123, 0)
	txid := []byte{0xaa, 0xbb}
	rows := []models.AddressTransaction{
		{
			Address:      "bc1qexample",
			Txid:         txid,
			BlockHeight:  100,
			TxIndex:      2,
			Role:         models.RoleReceiver,
			NetValueSats: 10,
			BlockTime:    blockTime,
		},
		{
			Address:      "bc1qexample",
			Txid:         txid,
			BlockHeight:  100,
			TxIndex:      2,
			Role:         models.RoleReceiver,
			NetValueSats: 25,
			BlockTime:    blockTime,
		},
		{
			Address:      "bc1qexample",
			Txid:         txid,
			BlockHeight:  100,
			TxIndex:      2,
			Role:         models.RoleSender,
			NetValueSats: -5,
			BlockTime:    blockTime,
		},
	}

	got := aggregateAddressTransactions(rows)
	if len(got) != 2 {
		t.Fatalf("expected 2 aggregated rows, got %d", len(got))
	}
	if got[0].NetValueSats != 35 {
		t.Fatalf("expected receiver net value 35, got %d", got[0].NetValueSats)
	}
	if got[1].NetValueSats != -5 {
		t.Fatalf("expected sender row to remain separate, got %d", got[1].NetValueSats)
	}
}

func TestFormatPartitionBoundMatchesMigrationNaming(t *testing.T) {
	cases := []struct {
		bound int32
		want  string
	}{
		{bound: 0, want: "0"},
		{bound: 100000, want: "100k"},
		{bound: 900000, want: "900k"},
		{bound: 1000000, want: "1m"},
		{bound: 1100000, want: "11m"},
	}

	for _, tc := range cases {
		got := formatPartitionBound(tc.bound)
		if got != tc.want {
			t.Fatalf("formatPartitionBound(%d) = %q, want %q", tc.bound, got, tc.want)
		}
	}
}

func TestPartitionBoundExprMatchesPostgresFormat(t *testing.T) {
	got := partitionBoundExpr(900000, 1000000)
	want := "FOR VALUES FROM (900000) TO (1000000)"
	if got != want {
		t.Fatalf("partitionBoundExpr() = %q, want %q", got, want)
	}
}
