package blockchain

import (
	"testing"

	"github.com/iPlatinuum/BockChain/crypto_utils"
	"github.com/iPlatinuum/BockChain/types"
	"github.com/stretchr/testify/assert"
)

// Add this method to your AccountState if not present
func (s *AccountState) Transfer(from, to types.Address, amount uint64) error {
	if err := s.Debit(from, amount); err != nil {
		return err
	}
	s.Credit(to, amount)
	return nil
}

func TestAccountState(t *testing.T) {
	state := NewAccountState()

	address := crypto_utils.GeneratePrivateKey().PublicKey().Address()
	account := state.CreateAccount(address)

	assert.Equal(t, account.Address, address)
	assert.Equal(t, account.Balance, uint64(0))

	fetchedAccount, err := state.GetAccount(address)
	assert.NoError(t, err)
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
	err := state.Transfer(addressBob, addressAlice, amount)
	assert.Error(t, err)
	assert.Equal(t, accountAlice.Balance, uint64(0))
}

func TestTransferSuccessEmptyToAccount(t *testing.T) {
	state := NewAccountState()

	addressBob := crypto_utils.GeneratePrivateKey().PublicKey().Address()
	addressAlice := crypto_utils.GeneratePrivateKey().PublicKey().Address()

	accountBob := state.CreateAccount(addressBob)
	accountBob.Balance = 100

	accountAlice := state.CreateAccount(addressAlice)

	amount := uint64(100)
	err := state.Transfer(addressBob, addressAlice, amount)
	assert.NoError(t, err)
	assert.Equal(t, accountAlice.Balance, amount)
}

func TestCreditAndDebit(t *testing.T) {
	s := NewAccountState()
	addr := types.AddressFromBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	s.CreateAccount(addr)

	s.Credit(addr, 100)
	err := s.Debit(addr, 50)
	if err != nil {
		t.Errorf("Expected debit to succeed, got error: %v", err)
	}
	err = s.Debit(addr, 100)
	if err == nil {
		t.Error("Expected insufficient balance error, got nil")
	}
}

func TestTransferWithFee(t *testing.T) {
	s := NewAccountState()
	from := types.AddressFromBytes([]byte{21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
		31, 32, 33, 34, 35, 36, 37, 38, 39, 40})
	to := types.AddressFromBytes([]byte{41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
		51, 52, 53, 54, 55, 56, 57, 58, 59, 60})
	fee := types.AddressFromBytes([]byte{61, 62, 63, 64, 65, 66, 67, 68, 69, 70,
		71, 72, 73, 74, 75, 76, 77, 78, 79, 80})
	s.CreateAccount(from)
	s.Credit(from, 100)

	err := s.TransferWithFee(from, to, fee, 70, 20)
	if err != nil {
		t.Errorf("Expected transfer to succeed, got error: %v", err)
	}
	if s.accounts[to].Balance != 70 {
		t.Errorf("Recipient balance should be 70, got %d", s.accounts[to].Balance)
	}
	if s.accounts[fee].Balance != 20 {
		t.Errorf("Fee recipient balance should be 20, got %d", s.accounts[fee].Balance)
	}
	if s.accounts[from].Balance != 10 {
		t.Errorf("Sender balance should be 10, got %d", s.accounts[from].Balance)
	}
}
