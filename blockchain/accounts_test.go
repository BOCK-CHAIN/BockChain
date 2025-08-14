package blockchain

import (
	"testing"

	"github.com/iPlatinuum/BockChain/crypto_utils"
	"github.com/stretchr/testify/assert"
)

func TestAccounState(t *testing.T) {
	state := NewAccountState()

	address := crypto_utils.GeneratePrivateKey().PublicKey().Address()
	account := state.CreateAccount(address)

	assert.Equal(t, account.Address, address)
	assert.Equal(t, account.Balance, uint64(0))

	fetchedAccount, err := state.GetAccount(address)
	assert.Nil(t, err)
	assert.Equal(t, fetchedAccount, account)
}

func TestTransferFailInsufficientBalance(t *testing.T) {
	state := NewAccountState()

	addressBob := crypto_utils.GeneratePrivateKey().PublicKey().Address()
	addressAlice := crypto_utils.GeneratePrivateKey().PublicKey().Address()

	accountBob := state.CreateAccount(addressBob)
	accountBob.Balance = 99

	accountAlice := state.CreateAccount(addressAlice)

	amount := uint64(100)
	assert.NotNil(t, state.Transfer(addressBob, addressAlice, amount))
	assert.Equal(t, accountAlice.Balance, uint64(0))
}

func TestTransferSuccessEmpyToAccount(t *testing.T) {
	state := NewAccountState()

	addressBob := crypto_utils.GeneratePrivateKey().PublicKey().Address()
	addressAlice := crypto_utils.GeneratePrivateKey().PublicKey().Address()

	accountBob := state.CreateAccount(addressBob)
	accountBob.Balance = 100

	accountAlice := state.CreateAccount(addressAlice)

	amount := uint64(100)
	assert.Nil(t, state.Transfer(addressBob, addressAlice, amount))
	assert.Equal(t, accountAlice.Balance, amount)
}


