package db

import (
	"testing"
	"time"

	"github.com/Abhinav7903/bitcoin-indexer/internal/models"
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
