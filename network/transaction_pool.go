package network

import (
	"sync"

	"github.com/iPlatinuum/BockChain/blockchain"
	"github.com/iPlatinuum/BockChain/types"
)

type TxPool struct {
	all       *TxSortedMap
	pending   *TxSortedMap
	maxLength int // Max number of transactions in pool
}

func NewTxPool(maxLength int) *TxPool {
	return &TxPool{
		all:       NewTxSortedMap(),
		pending:   NewTxSortedMap(),
		maxLength: maxLength,
	}
}

func (p *TxPool) Add(tx *blockchain.Transaction) {
	// If pool is full, prune oldest tx
	if p.all.Count() == p.maxLength {
		oldest := p.all.First()
		p.all.Remove(oldest.Hash(blockchain.TxHasher{}))
	}

	if !p.all.Contains(tx.Hash(blockchain.TxHasher{})) {
		p.all.Add(tx)
		p.pending.Add(tx)
	}
}

func (p *TxPool) Contains(hash types.Hash) bool {
	return p.all.Contains(hash)
}

// Pending returns all transactions currently in the pending pool.
func (p *TxPool) Pending() []*blockchain.Transaction {
	return p.pending.txx.Data
}

func (p *TxPool) ClearPending() {
	p.pending.Clear()
}

func (p *TxPool) PendingCount() int {
	return p.pending.Count()
}

// ----------------------------------------------------

type TxSortedMap struct {
	lock   sync.RWMutex
	lookup map[types.Hash]*blockchain.Transaction
	txx    *types.List[*blockchain.Transaction]
}

func NewTxSortedMap() *TxSortedMap {
	return &TxSortedMap{
		lookup: make(map[types.Hash]*blockchain.Transaction),
		txx:    types.NewList[*blockchain.Transaction](),
	}
}

func (t *TxSortedMap) First() *blockchain.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	first := t.txx.Get(0)
	return t.lookup[first.Hash(blockchain.TxHasher{})]
}

func (t *TxSortedMap) Get(h types.Hash) *blockchain.Transaction {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return t.lookup[h]
}

func (t *TxSortedMap) Add(tx *blockchain.Transaction) {
	hash := tx.Hash(blockchain.TxHasher{})

	t.lock.Lock()
	defer t.lock.Unlock()

	if _, ok := t.lookup[hash]; !ok {
		t.lookup[hash] = tx
		t.txx.Insert(tx)
	}
}

func (t *TxSortedMap) Remove(h types.Hash) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.txx.Remove(t.lookup[h])
	delete(t.lookup, h)
}

func (t *TxSortedMap) Count() int {
	t.lock.RLock()
	defer t.lock.RUnlock()

	return len(t.lookup)
}

func (t *TxSortedMap) Contains(h types.Hash) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()

	_, ok := t.lookup[h]
	return ok
}

func (t *TxSortedMap) Clear() {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.lookup = make(map[types.Hash]*blockchain.Transaction)
	t.txx.Clear()
}
