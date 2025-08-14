package util

import (
	"math/rand"
	"testing"
	"time"

	"github.com/iPlatinuum/BockChain/blockchain"
	"github.com/iPlatinuum/BockChain/crypto_utils"
	"github.com/iPlatinuum/BockChain/types"
	"github.com/stretchr/testify/assert"
)

func RandomBytes(size int) []byte {
	token := make([]byte, size)
	rand.Read(token)
	return token
}

func RandomHash() types.Hash {
	return types.HashFromBytes(RandomBytes(32))
}

// NewRandomTransaction returns a new random transaction without signature.
func NewRandomTransaction(size int) *blockchain.Transaction {
	return blockchain.NewTransaction(RandomBytes(size))
}

func NewRandomTransactionWithSignature(t *testing.T, privKey crypto_utils.PrivateKey, size int) *blockchain.Transaction {
	tx := NewRandomTransaction(size)
	assert.Nil(t, tx.Sign(privKey))
	return tx
}

func NewRandomBlock(t *testing.T, height uint32, prevBlockHash types.Hash) *blockchain.Block {
	txSigner := crypto_utils.GeneratePrivateKey()
	tx := NewRandomTransactionWithSignature(t, txSigner, 100)
	header := &blockchain.Header{
		Version:       1,
		PrevBlockHash: prevBlockHash,
		Height:        height,
		Timestamp:     time.Now().UnixNano(),
	}
	b, err := blockchain.NewBlock(header, []*blockchain.Transaction{tx})
	assert.Nil(t, err)

	dataHash, err := blockchain.CalculateDataHash(b.Transactions)
	assert.Nil(t, err)
	b.Header.DataHash = dataHash

	return b
}

func NewRandomBlockWithSignature(t *testing.T, pk crypto_utils.PrivateKey, height uint32, prevHash types.Hash) *blockchain.Block {
	b := NewRandomBlock(t, height, prevHash)
	assert.Nil(t, b.Sign(pk))
	return b
}
