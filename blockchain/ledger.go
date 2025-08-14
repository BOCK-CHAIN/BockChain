package blockchain

import (
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/iPlatinuum/BockChain/crypto_utils"
	"github.com/iPlatinuum/BockChain/types"
)

type Blockchain struct {
	logger     log.Logger
	store      Storage
	lock       sync.RWMutex
	headers    []*Header
	blocks     []*Block
	txStore    map[types.Hash]*Transaction
	blockStore map[types.Hash]*Block

	accountState *AccountState

	stateLock       sync.RWMutex
	collectionState map[types.Hash]*CollectionTx
	mintState       map[types.Hash]*MintTx
	validator       Validator
	contractState   *State
}

func NewBlockchain(l log.Logger, genesis *Block) (*Blockchain, error) {
	accountState := NewAccountState()
	coinbase := crypto_utils.PublicKey{}
	accountState.CreateAccount(coinbase.Address())

	bc := &Blockchain{
		contractState:   NewState(),
		headers:         []*Header{},
		store:           NewMemorystore(),
		logger:          l,
		accountState:    accountState,
		collectionState: make(map[types.Hash]*CollectionTx),
		mintState:       make(map[types.Hash]*MintTx),
		blockStore:      make(map[types.Hash]*Block),
		txStore:         make(map[types.Hash]*Transaction),
	}
	bc.validator = NewBlockValidator(bc)
	err := bc.addBlockWithoutValidation(genesis)
	return bc, err
}

func (bc *Blockchain) SetValidator(v Validator) {
	bc.validator = v
}

func (bc *Blockchain) AddBlock(b *Block) error {
	if err := bc.validator.ValidateBlock(b); err != nil {
		return err
	}
	return bc.addBlockWithoutValidation(b)
}

func (bc *Blockchain) handleNativeTransfer(tx *Transaction) error {
	fmt.Printf("[SMOOTH-LOG] Native token transfer: from=%s to=%s value=%d\n",
		tx.From, tx.To, tx.Value)
	return bc.accountState.Transfer(tx.From.Address(), tx.To.Address(), tx.Value)
}

func (bc *Blockchain) handleNativeNFT(tx *Transaction) error {
	hash := tx.Hash(TxHasher{})
	switch t := tx.TxInner.(type) {
	case CollectionTx:
		bc.collectionState[hash] = &t
		fmt.Printf("[SMOOTH-LOG] Created new NFT collection hash=%s\n", hash)
	case MintTx:
		_, ok := bc.collectionState[t.Collection]
		if !ok {
			return fmt.Errorf("collection (%s) does not exist on the blockchain", t.Collection)
		}
		bc.mintState[hash] = &t
		fmt.Printf("[SMOOTH-LOG] Minted NFT %s in collection %s\n", t.NFT, t.Collection)
	default:
		return fmt.Errorf("unsupported tx type %v", t)
	}
	return nil
}

func (bc *Blockchain) GetBlockByHash(hash types.Hash) (*Block, error) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	block, ok := bc.blockStore[hash]
	if !ok {
		return nil, fmt.Errorf("block with hash (%s) not found", hash)
	}
	return block, nil
}

func (bc *Blockchain) GetBlock(height uint32) (*Block, error) {
	if height > bc.Height() {
		return nil, fmt.Errorf("given height (%d) too high", height)
	}
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.blocks[height], nil
}

func (bc *Blockchain) GetHeader(height uint32) (*Header, error) {
	if height > bc.Height() {
		return nil, fmt.Errorf("given height (%d) too high", height)
	}
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.headers[height], nil
}

func (bc *Blockchain) GetTxByHash(hash types.Hash) (*Transaction, error) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	tx, ok := bc.txStore[hash]
	if !ok {
		return nil, fmt.Errorf("could not find tx with hash (%s)", hash)
	}
	return tx, nil
}

func (bc *Blockchain) HasBlock(height uint32) bool {
	return height <= bc.Height()
}

func (bc *Blockchain) Height() uint32 {
	bc.lock.RLock()
	defer bc.lock.RUnlock()
	return uint32(len(bc.headers) - 1)
}

func (bc *Blockchain) handleTransaction(tx *Transaction) error {
	if len(tx.Data) > 0 {
		fmt.Printf("[SMOOTH-LOG] Executing contract code len=%d hash=%s\n",
			len(tx.Data), tx.Hash(&TxHasher{}))
		vm := NewVM(tx.Data, bc.contractState)
		if err := vm.Run(); err != nil {
			return err
		}
	}
	if tx.TxInner != nil {
		if err := bc.handleNativeNFT(tx); err != nil {
			return err
		}
	}
	if tx.Value > 0 {
		if err := bc.handleNativeTransfer(tx); err != nil {
			return err
		}
	}
	return nil
}

func (bc *Blockchain) addBlockWithoutValidation(b *Block) error {
	bc.stateLock.Lock()
	for i := 0; i < len(b.Transactions); i++ {
		if err := bc.handleTransaction(b.Transactions[i]); err != nil {
			fmt.Printf("[SMOOTH-LOG] Error processing tx: %v\n", err)
			b.Transactions[i] = b.Transactions[len(b.Transactions)-1]
			b.Transactions = b.Transactions[:len(b.Transactions)-1]
			continue
		}
	}
	bc.stateLock.Unlock()

	bc.lock.Lock()
	bc.headers = append(bc.headers, b.Header)
	bc.blocks = append(bc.blocks, b)
	bc.blockStore[b.Hash(BlockHasher{})] = b
	for _, tx := range b.Transactions {
		bc.txStore[tx.Hash(TxHasher{})] = tx
	}
	bc.lock.Unlock()

	fmt.Printf("[SMOOTH-LOG] New block added: hash=%s height=%d transactions=%d\n",
		b.Hash(BlockHasher{}), b.Height, len(b.Transactions))

	return bc.store.Put(b)
}
