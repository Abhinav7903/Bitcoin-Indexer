package api

import "time"

type AddressInfo struct {
	Address           string `json:"address"`
	BalanceSats       int64  `json:"balance_sats"`
	TotalReceivedSats int64  `json:"total_received_sats"`
	TotalSentSats     int64  `json:"total_sent_sats"`
	TxCount           int    `json:"tx_count"`
	UtxoCount         int    `json:"utxo_count"`
	FirstSeenHeight   int32  `json:"first_seen_height"`
	LastSeenHeight    int32  `json:"last_seen_height"`
}

type AddressTransaction struct {
	Txid         string    `json:"txid"`
	BlockHeight  int32     `json:"block_height"`
	BlockTime    time.Time `json:"block_time"`
	NetValueSats int64     `json:"net_value_sats"`
	Role         string    `json:"role"` // "sender", "receiver", "both"
}

type TxInfo struct {
	Txid         string      `json:"txid"`
	BlockHeight  int32       `json:"block_height"`
	BlockHash    string      `json:"block_hash"`
	BlockTime    time.Time   `json:"block_time"`
	Version      int32       `json:"version"`
	Locktime     int64       `json:"locktime"`
	IsCoinbase   bool        `json:"is_coinbase"`
	FeeSats      *int64      `json:"fee_sats"`
	SizeBytes    int32       `json:"size_bytes"`
	VSize        int32       `json:"vsize"`
	Weight       int32       `json:"weight"`
	Inputs       []TxInput   `json:"inputs"`
	Outputs      []TxOutput  `json:"outputs"`
}

type TxInput struct {
	Index      int    `json:"index"`
	PrevTxid   string `json:"prev_txid,omitempty"`
	PrevVout   *int   `json:"prev_vout,omitempty"`
	ValueSats  int64  `json:"value_sats"`
	Address    string `json:"address,omitempty"`
	ScriptSig  string `json:"script_sig,omitempty"`
	SequenceNo int64  `json:"sequence_no"`
}

type TxOutput struct {
	Index        int    `json:"index"`
	ValueSats    int64  `json:"value_sats"`
	Address      string `json:"address,omitempty"`
	ScriptPubKey string `json:"script_pubkey,omitempty"`
	ScriptType   string `json:"script_type"`
	Spent        bool   `json:"spent"`
	SpendingTxid string `json:"spending_txid,omitempty"`
}
