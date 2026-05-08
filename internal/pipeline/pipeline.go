package pipeline

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Abhinav7903/bitcoin-indexer/internal/db"
	"github.com/Abhinav7903/bitcoin-indexer/internal/models"
	"github.com/Abhinav7903/bitcoin-indexer/pkg/rpc"
)

type Pipeline struct {
	rpcClient *rpc.Client
	dbWriter  *db.Writer
	workers   int
	batchSize int
}

type blockResult struct {
	block   models.Block
	txs     []models.Transaction
	outputs []models.Output
	addrTxs []models.AddressTransaction
	inputs  []models.Input
	err     error
}

func NewPipeline(rpcClient *rpc.Client, dbWriter *db.Writer, workers, batchSize int) *Pipeline {
	if workers < 1 {
		workers = 1
	}
	if batchSize < 1 {
		batchSize = 1
	}
	return &Pipeline{rpcClient: rpcClient, dbWriter: dbWriter, workers: workers, batchSize: batchSize}
}

func (p *Pipeline) Run(ctx context.Context, startHeight int32) error {

	var (
		cachedInfo     *rpc.BlockchainInfo
		lastInfoUpdate time.Time
	)

	const safeConfirmations int32 = 10

	for {

		// ----------------------------------------
		// Refresh blockchain info every 5 seconds
		// ----------------------------------------

		if cachedInfo == nil ||
			time.Since(lastInfoUpdate) > 5*time.Second {

			info, err := p.rpcClient.GetBlockchainInfo()
			if err != nil {

				log.Printf(
					"Failed to fetch blockchain info: %v",
					err,
				)

				select {

				case <-ctx.Done():
					return ctx.Err()

				case <-time.After(15 * time.Second):
					continue
				}
			}

			cachedInfo = info
			lastInfoUpdate = time.Now()

			log.Printf(
				"RPC blockchain info | blocks=%d headers=%d ibd=%v",
				info.Blocks,
				info.Headers,
				info.InitialBlockDownload,
			)

			if info.Blocks < info.Headers {

				log.Printf(
					"Bitcoin node syncing | blocks=%d headers=%d remaining=%d",
					info.Blocks,
					info.Headers,
					info.Headers-info.Blocks,
				)
			}
		}

		blocks := int32(cachedInfo.Blocks)
		headers := int32(cachedInfo.Headers)

		// ----------------------------------------
		// Always stay behind tip
		// ----------------------------------------

		safeTip := blocks - safeConfirmations

		if safeTip < 0 {
			safeTip = 0
		}

		// ----------------------------------------
		// Indexer caught up
		// ----------------------------------------

		if startHeight > safeTip {

			log.Printf(
				"Indexer caught up | start=%d safe_tip=%d waiting for more blocks...",
				startHeight,
				safeTip,
			)

			select {

			case <-ctx.Done():
				return ctx.Err()

			case <-time.After(10 * time.Second):
				continue
			}
		}

		// ----------------------------------------
		// Batch range
		// ----------------------------------------

		endHeight := startHeight + int32(p.batchSize) - 1

		if endHeight > safeTip {
			endHeight = safeTip
		}

		log.Printf(
			"Ingesting blocks %d -> %d | safe_tip=%d blocks=%d headers=%d",
			startHeight,
			endHeight,
			safeTip,
			blocks,
			headers,
		)

		// ----------------------------------------
		// Ingest batch
		// ----------------------------------------

		if err := p.ingestRange(
			ctx,
			startHeight,
			endHeight,
		); err != nil {

			log.Printf(
				"Ingestion error (%d-%d): %v",
				startHeight,
				endHeight,
				err,
			)

			log.Printf(
				"Retrying in 15 seconds...",
			)

			select {

			case <-ctx.Done():
				return ctx.Err()

			case <-time.After(15 * time.Second):
				continue
			}
		}

		startHeight = endHeight + 1
	}
}
func (p *Pipeline) ingestRange(ctx context.Context, start, end int32) error {
	count := int(end - start + 1)
	startTime := time.Now()

	resChan := make(chan blockResult, count)
	heightChan := make(chan int32, count)

	workerCount := p.workers
	if workerCount > count {
		workerCount = count
	}

	for i := 0; i < workerCount; i++ {
		go func() {
			for h := range heightChan {
				select {
				case <-ctx.Done():
					return
				case resChan <- p.fetchBlock(h):
				}
			}
		}()
	}

	for h := start; h <= end; h++ {
		heightChan <- h
	}

	close(heightChan)

	results := make([]blockResult, 0, count)

	var firstErr error

	for i := 0; i < count; i++ {
		res := <-resChan

		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}

		results = append(results, res)
	}

	if firstErr != nil {
		return firstErr
	}

	fetchDuration := time.Since(startTime)

	sort.Slice(results, func(i, j int) bool {
		return results[i].block.Height < results[j].block.Height
	})

	var blocks []models.Block
	var txs []models.Transaction
	var outputs []models.Output
	var addrTxs []models.AddressTransaction
	var inputs []models.Input

	totalTxs := 0
	for _, res := range results {
		blocks = append(blocks, res.block)
		txs = append(txs, res.txs...)
		outputs = append(outputs, res.outputs...)
		addrTxs = append(addrTxs, res.addrTxs...)
		inputs = append(inputs, res.inputs...)
		totalTxs += len(res.txs)
	}

	dbStartTime := time.Now()
	err := p.dbWriter.SaveBlockBatch(
		ctx,
		blocks,
		txs,
		outputs,
		addrTxs,
		inputs,
	)
	dbDuration := time.Since(dbStartTime)

	if err == nil {
		log.Printf("Batch %d-%d: Fetched %d blocks (%d txs) in %v, DB write in %v",
			start, end, count, totalTxs, fetchDuration, dbDuration)
	}

	return err
}

func (p *Pipeline) fetchBlock(height int32) blockResult {
	rpcStart := time.Now()
	hash, err := p.rpcClient.GetBlockHash(height)
	if err != nil {
		return blockResult{err: err}
	}
	rawBlock, err := p.rpcClient.GetBlockVerbose(hash)
	if err != nil {
		return blockResult{err: err}
	}
	rpcDuration := time.Since(rpcStart)

	parseStart := time.Now()
	res := parseBlock(height, rawBlock)
	parseDuration := time.Since(parseStart)

	if res.err == nil {
		// Only log if it's taking significant time, or if you want to see every block
		if rpcDuration > 500*time.Millisecond || parseDuration > 100*time.Millisecond {
			log.Printf("Block %d: RPC %v, Parse %v (%d txs)", height, rpcDuration, parseDuration, len(res.txs))
		}
	}

	return res
}

func parseBlock(height int32, rawBlock map[string]interface{}) blockResult {

	blockHash, err := hexString(rawBlock, "hash")
	if err != nil {
		return blockResult{err: err}
	}

	blockTime := time.Unix(
		int64(number(rawBlock["time"])),
		0,
	)

	txList, ok := rawBlock["tx"].([]interface{})
	if !ok {
		return blockResult{
			err: fmt.Errorf(
				"block %d has invalid tx list",
				height,
			),
		}
	}

	// ----------------------------------------
	// Genesis block fix
	// ----------------------------------------

	prevHash := optionalHex(
		rawBlock["previousblockhash"],
	)

	// Genesis block has no previous hash.
	// Store 32-byte zero hash instead of NULL.
	if height == 0 && len(prevHash) == 0 {
		prevHash = make([]byte, 32)
	}

	block := models.Block{
		Hash:          blockHash,
		Height:        height,
		PreviousHash:  prevHash,
		MerkleRoot:    optionalHex(rawBlock["merkleroot"]),
		Time:          blockTime,
		Bits:          bitsToInt(rawBlock["bits"]),
		Nonce:         int64(number(rawBlock["nonce"])),
		Version:       int32(number(rawBlock["version"])),
		TxCount:       int32(len(txList)),
		SizeBytes:     int32(number(rawBlock["size"])),
		Weight:        int32(number(rawBlock["weight"])),
		TotalFeesSats: 0,
	}

	var blockTxs []models.Transaction
	var blockOutputs []models.Output
	var blockAddrTxs []models.AddressTransaction
	var blockInputs []models.Input

	for txIndex, item := range txList {

		rawTx, ok := item.(map[string]interface{})
		if !ok {
			return blockResult{
				err: fmt.Errorf(
					"block %d tx %d has invalid shape",
					height,
					txIndex,
				),
			}
		}

		txid, err := hexString(rawTx, "txid")
		if err != nil {
			return blockResult{err: err}
		}

		vins := list(rawTx["vin"])
		vouts := list(rawTx["vout"])

		isCoinbase := len(vins) > 0 &&
			hasKey(vins[0], "coinbase")

		fee := optionalSats(rawTx["fee"])

		if fee != nil {
			block.TotalFeesSats += *fee
		}

		blockTxs = append(
			blockTxs,
			models.Transaction{
				Txid:        txid,
				BlockHash:   blockHash,
				BlockHeight: height,
				TxIndex:     int32(txIndex),
				Version:     int32(number(rawTx["version"])),
				Locktime:    int64(number(rawTx["locktime"])),
				IsCoinbase:  isCoinbase,
				InputCount:  int16(len(vins)),
				OutputCount: int16(len(vouts)),
				FeeSats:     fee,
				SizeBytes:   int32(number(rawTx["size"])),
				VSize:       int32(number(rawTx["vsize"])),
				Weight:      int32(number(rawTx["weight"])),
				HasSegwit:   txHasWitness(vins),
			},
		)

		// ----------------------------------------
		// Inputs
		// ----------------------------------------

		for vinIndex, item := range vins {

			vin := asMap(item)

			input := models.Input{
				Txid:        txid,
				VinIdx:      int32(vinIndex),
				BlockHeight: height,
				ScriptSig:   scriptSig(vin),
				WitnessData: witness(vin),
				SequenceNo: int64(
					numberDefault(
						vin["sequence"],
						4294967294,
					),
				),
			}

			if !hasKey(item, "coinbase") {

				input.PrevTxid = optionalHex(
					vin["txid"],
				)

				prevVout := int32(
					number(vin["vout"]),
				)

				input.PrevVout = &prevVout
			}

			blockInputs = append(
				blockInputs,
				input,
			)
		}

		// ----------------------------------------
		// Outputs
		// ----------------------------------------

		for _, item := range vouts {

			vout := asMap(item)
			spk := asMap(vout["scriptPubKey"])

			addr := scriptAddress(spk)
			value := sats(vout["value"])

			output := models.Output{
				Txid:         txid,
				VoutIdx:      int32(number(vout["n"])),
				Address:      addr,
				ValueSats:    value,
				BlockHeight:  height,
				ScriptPubKey: optionalHex(spk["hex"]),
				ScriptType:   scriptType(spk),
			}

			blockOutputs = append(
				blockOutputs,
				output,
			)

			if addr != "" {

				blockAddrTxs = append(
					blockAddrTxs,
					models.AddressTransaction{
						Address:      addr,
						Txid:         txid,
						BlockHeight:  height,
						TxIndex:      int32(txIndex),
						Role:         models.RoleReceiver,
						NetValueSats: value,
						BlockTime:    blockTime,
					},
				)
			}
		}
	}

	return blockResult{
		block:   block,
		txs:     blockTxs,
		outputs: blockOutputs,
		addrTxs: blockAddrTxs,
		inputs:  blockInputs,
	}
}

func hexString(m map[string]interface{}, key string) ([]byte, error) {
	value, ok := m[key].(string)
	if !ok || value == "" {
		return nil, fmt.Errorf("missing hex string field %q", key)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", key, err)
	}
	return decoded, nil
}

func optionalHex(value interface{}) []byte {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return decoded
}

func number(value interface{}) float64 {
	return numberDefault(value, 0)
}

func numberDefault(value interface{}, fallback float64) float64 {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f
		}
	case float64:
		return v
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}

func sats(value interface{}) int64 {
	return int64(math.Round(number(value) * 1e8))
}

func optionalSats(value interface{}) *int64 {
	if value == nil {
		return nil
	}
	s := sats(value)
	return &s
}

func bitsToInt(value interface{}) int64 {
	s, ok := value.(string)
	if !ok || s == "" {
		return int64(number(value))
	}
	parsed, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func list(value interface{}) []interface{} {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	return items
}

func asMap(value interface{}) map[string]interface{} {
	m, _ := value.(map[string]interface{})
	return m
}

func hasKey(value interface{}, key string) bool {
	m := asMap(value)
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func scriptAddress(spk map[string]interface{}) string {
	if spk == nil {
		return ""
	}
	if address, ok := spk["address"].(string); ok {
		return address
	}
	addresses := list(spk["addresses"])
	if len(addresses) > 0 {
		if address, ok := addresses[0].(string); ok {
			return address
		}
	}
	return ""
}

func scriptType(spk map[string]interface{}) int16 {
	if spk == nil {
		return models.ScriptUnknown
	}
	typeName, _ := spk["type"].(string)
	switch strings.ToLower(typeName) {
	case "pubkeyhash", "p2pkh":
		return models.ScriptP2PKH
	case "scripthash", "p2sh":
		return models.ScriptP2SH
	case "witness_v0_keyhash", "p2wpkh":
		return models.ScriptP2WPKH
	case "witness_v0_scripthash", "p2wsh":
		return models.ScriptP2WSH
	case "witness_v1_taproot", "p2tr":
		return models.ScriptP2TR
	case "nulldata", "op_return":
		return models.ScriptOpReturn
	case "multisig":
		return models.ScriptMultisig
	default:
		return models.ScriptUnknown
	}
}

func scriptSig(vin map[string]interface{}) []byte {
	sig := asMap(vin["scriptSig"])
	if sig == nil {
		return nil
	}
	return optionalHex(sig["hex"])
}

func witness(vin map[string]interface{}) [][]byte {
	items := list(vin["txinwitness"])
	if len(items) == 0 {
		return nil
	}
	decoded := make([][]byte, 0, len(items))
	for _, item := range items {
		decoded = append(decoded, optionalHex(item))
	}
	return decoded
}

func txHasWitness(vins []interface{}) bool {
	for _, item := range vins {
		if len(list(asMap(item)["txinwitness"])) > 0 {
			return true
		}
	}
	return false
}
