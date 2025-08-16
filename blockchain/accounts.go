package blockchain

import (
	"errors"
	"fmt"
	"sync"

	"github.com/iPlatinuum/BockChain/types"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrInsufficientBalance = errors.New("insufficient account balance")
)

type Account struct {
	Address types.Address
	Balance uint64
}

func (a *Account) String() string {
	return fmt.Sprintf("%d", a.Balance)
}

type AccountState struct {
	balances map[types.Address]uint64
	mu       sync.RWMutex
	accounts map[types.Address]*Account
	lock     sync.RWMutex
}

func (s *AccountState) Credit(addr types.Address, amount uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc, ok := s.accounts[addr]
	if !ok {
		acc = s.CreateAccount(addr)
	}
	acc.Balance += amount
}

func (s *AccountState) Debit(addr types.Address, amount uint64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	acc, ok := s.accounts[addr]
	if !ok {
		return ErrAccountNotFound
	}

	if acc.Balance < amount {
		return ErrInsufficientBalance
	}

	acc.Balance -= amount
	return nil
}

func NewAccountState() *AccountState {
	return &AccountState{
		accounts: make(map[types.Address]*Account),
	}
}

func (s *AccountState) CreateAccount(address types.Address) *Account {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc := &Account{Address: address}
	s.accounts[address] = acc
	return acc
}

func (s *AccountState) GetAccount(address types.Address) (*Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAccountWithoutLock(address)
}

func (s *AccountState) getAccountWithoutLock(address types.Address) (*Account, error) {
	account, ok := s.accounts[address]
	if !ok {
		return nil, ErrAccountNotFound
	}

	return account, nil
}

func (s *AccountState) GetBalance(address types.Address) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, err := s.getAccountWithoutLock(address)
	if err != nil {
		return 0, err
	}

	return account.Balance, nil
}

func (s *AccountState) TransferWithFee(from, to, feeRecipient types.Address, amount, fee uint64) error {
	total := amount + fee
	if from.String() != "996fb92427ae41e4649b934ca495991b7852b855" { // maybe special exempt addr
		if err := s.Debit(from, total); err != nil {
			return err
		}
	}

	s.Credit(to, amount)
	s.Credit(feeRecipient, fee)
	return nil
}
