package blockchain

import (
	"errors"
	"fmt"
)

var ErrBlockKnown = errors.New("block already known")

type Validator interface {
	ValidateBlock(*Block) error
}

type BlockValidator struct {
	bc *Blockchain
}

func NewBlockValidator(bc *Blockchain) *BlockValidator {
	return &BlockValidator{
		bc: bc,
	}
}

func (v *BlockValidator) ValidateBlock(b *Block) error {
	// 1. Check for duplicate block
	if v.bc.HasBlock(b.Height) {
		fmt.Printf("[SMOOTH-LOG] Block %s (height=%d) already known — skipping\n",
			b.Hash(BlockHasher{}), b.Height)
		return ErrBlockKnown
	}

	// 2. Height must be sequential
	if b.Height != v.bc.Height()+1 {
		fmt.Printf("[SMOOTH-LOG] Rejected block %s: height=%d is too high (current height=%d)\n",
			b.Hash(BlockHasher{}), b.Height, v.bc.Height())
		return fmt.Errorf("block (%s) with height (%d) is too high => current height (%d)",
			b.Hash(BlockHasher{}), b.Height, v.bc.Height())
	}

	// 3. Check previous block hash matches expected
	prevHeader, err := v.bc.GetHeader(b.Height - 1)
	if err != nil {
		fmt.Printf("[SMOOTH-LOG] Failed to get header for height %d: %v\n", b.Height-1, err)
		return err
	}
	expectedHash := BlockHasher{}.Hash(prevHeader)
	if expectedHash != b.PrevBlockHash {
		fmt.Printf("[SMOOTH-LOG] Rejected block %s: invalid prev block hash (got=%s expected=%s)\n",
			b.Hash(BlockHasher{}), b.PrevBlockHash, expectedHash)
		return fmt.Errorf("the hash of the previous block (%s) is invalid", b.PrevBlockHash)
	}

	// 4. Verify block’s own integrity
	if err := b.Verify(); err != nil {
		fmt.Printf("[SMOOTH-LOG] Rejected block %s: verification failed — %v\n",
			b.Hash(BlockHasher{}), err)
		return err
	}

	// Passed all checks
	fmt.Printf("[SMOOTH-LOG] Block %s (height=%d) is valid and ready to be added\n",
		b.Hash(BlockHasher{}), b.Height)
	return nil
}
