package models

import "time"

const (
	RoleReceiver int16 = 0
	RoleSender   int16 = 1
	RoleBoth     int16 = 2
)

const (
	ScriptP2PKH    int16 = 0
	ScriptP2SH     int16 = 1
	ScriptP2WPKH   int16 = 2
	ScriptP2WSH    int16 = 3
	ScriptP2TR     int16 = 4
	ScriptOpReturn int16 = 5
	ScriptMultisig int16 = 6
	ScriptUnknown  int16 = 7
)

type Block struct {
	Hash          []byte
	Height        int32
	PreviousHash  []byte
	MerkleRoot    []byte
	Time          time.Time
	Bits          int64
	Nonce         int64
	Version       int32
	TxCount       int32
	SizeBytes     int32
	Weight        int32
	TotalFeesSats int64
}

type Transaction struct {
	Txid        []byte
	BlockHash   []byte
	BlockHeight int32
	TxIndex     int32
	Version     int32
	Locktime    int64
	IsCoinbase  bool
	InputCount  int16
	OutputCount int16
	FeeSats     *int64
	SizeBytes   int32
	VSize       int32
	Weight      int32
	HasSegwit   bool
}

type Output struct {
	Txid         []byte
	VoutIdx      int32
	Address      string
	ValueSats    int64
	BlockHeight  int32
	ScriptPubKey []byte
	ScriptType   int16
}

type Input struct {
	Txid        []byte
	VinIdx      int32
	PrevTxid    []byte
	PrevVout    *int32
	BlockHeight int32
	ScriptSig   []byte
	WitnessData [][]byte
	SequenceNo  int64
}

type AddressTransaction struct {
	Address      string
	Txid         []byte
	BlockHeight  int32
	TxIndex      int32
	Role         int16
	NetValueSats int64
	BlockTime    time.Time
}
