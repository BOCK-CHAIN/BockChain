package network

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/iPlatinuum/BockChain/api"
	"github.com/iPlatinuum/BockChain/blockchain"
	"github.com/iPlatinuum/BockChain/crypto_utils"
	"github.com/iPlatinuum/BockChain/types"
)

var defaultBlockTime = 5 * time.Second

type ServerOpts struct {
	APIListenAddr string
	SeedNodes     []string
	ListenAddr    string
	TCPTransport  *TCPTransport
	ID            string
	Logger        log.Logger
	RPCDecodeFunc RPCDecodeFunc
	RPCProcessor  RPCProcessor
	BlockTime     time.Duration
	PrivateKey    *crypto_utils.PrivateKey
}

type Server struct {
	TCPTransport *TCPTransport
	peerCh       chan *TCPPeer

	mu      sync.RWMutex
	peerMap map[net.Addr]*TCPPeer

	ServerOpts
	mempool     *TxPool
	chain       *blockchain.Blockchain
	isValidator bool
	rpcCh       chan RPC
	quitCh      chan struct{}
	txChan      chan *blockchain.Transaction
}

func NewServer(opts ServerOpts) (*Server, error) {
	if opts.BlockTime == 0 {
		opts.BlockTime = defaultBlockTime
	}
	if opts.RPCDecodeFunc == nil {
		opts.RPCDecodeFunc = DefaultRPCDecodeFunc
	}
	if opts.Logger == nil {
		opts.Logger = log.NewLogfmtLogger(os.Stderr)
		opts.Logger = log.With(opts.Logger, "addr", opts.ID)
	}

	chain, err := blockchain.NewBlockchain(opts.Logger, genesisBlock())
	if err != nil {
		return nil, err
	}

	txChan := make(chan *blockchain.Transaction)

	if opts.APIListenAddr != "" {
		apiServerCfg := api.ServerConfig{
			Logger:     opts.Logger,
			ListenAddr: opts.APIListenAddr,
		}
		apiServer := api.NewServer(apiServerCfg, chain, txChan)
		go apiServer.Start()

		fmt.Printf("[SMOOTH-LOG] JSON API server running on port %s\n", opts.APIListenAddr)
	}

	peerCh := make(chan *TCPPeer)
	tr := NewTCPTransport(opts.ListenAddr, peerCh)

	s := &Server{
		TCPTransport: tr,
		peerCh:       peerCh,
		peerMap:      make(map[net.Addr]*TCPPeer),
		ServerOpts:   opts,
		chain:        chain,
		mempool:      NewTxPool(1000),
		isValidator:  opts.PrivateKey != nil,
		rpcCh:        make(chan RPC),
		quitCh:       make(chan struct{}, 1),
		txChan:       txChan,
	}

	s.TCPTransport.peerCh = peerCh

	if s.RPCProcessor == nil {
		s.RPCProcessor = s
	}

	if s.isValidator {
		go s.validatorLoop()
	}

	return s, nil
}

func (s *Server) bootstrapNetwork() {
	for _, addr := range s.SeedNodes {
		fmt.Printf("[SMOOTH-LOG] Trying to connect to %s\n", addr)

		go func(addr string) {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				fmt.Printf("[SMOOTH-LOG] Could not connect to %s: %v\n", addr, err)
				return
			}
			s.peerCh <- &TCPPeer{conn: conn}
		}(addr)
	}
}

func (s *Server) Start() {
	s.TCPTransport.Start()
	time.Sleep(time.Second)
	s.bootstrapNetwork()

	fmt.Printf("[SMOOTH-LOG] Accepting TCP connections on %s (Node ID: %s)\n", s.ListenAddr, s.ID)

free:
	for {
		select {
		case peer := <-s.peerCh:
			s.peerMap[peer.conn.RemoteAddr()] = peer
			go peer.readLoop(s.rpcCh)

			if err := s.sendGetStatusMessage(peer); err != nil {
				continue
			}

			fmt.Printf("[SMOOTH-LOG] Connected to peer at %s (outgoing: %v)\n", peer.conn.RemoteAddr(), peer.Outgoing)

		case tx := <-s.txChan:
			if err := s.processTransaction(tx); err != nil {
				fmt.Printf("[SMOOTH-LOG] Error processing transaction: %v\n", err)
			}

		case rpc := <-s.rpcCh:
			msg, err := s.RPCDecodeFunc(rpc)
			if err != nil {
				fmt.Printf("[SMOOTH-LOG] Failed to decode RPC: %v\n", err)
				continue
			}

			if err := s.RPCProcessor.ProcessMessage(msg); err != nil && err != blockchain.ErrBlockKnown {
				fmt.Printf("[SMOOTH-LOG] Error processing RPC message: %v\n", err)
			}

		case <-s.quitCh:
			break free
		}
	}

	fmt.Printf("[SMOOTH-LOG] Server is shutting down\n")
}

func (s *Server) validatorLoop() {
	ticker := time.NewTicker(s.BlockTime)
	fmt.Printf("[SMOOTH-LOG] Starting validator loop, blockTime=%v\n", s.BlockTime)

	for {
		fmt.Printf("[SMOOTH-LOG] Creating new block...\n")

		if err := s.createNewBlock(); err != nil {
			fmt.Printf("[SMOOTH-LOG] Error creating new block: %v\n", err)
		}

		<-ticker.C
	}
}

func (s *Server) ProcessMessage(msg *DecodedMessage) error {
	switch t := msg.Data.(type) {
	case *blockchain.Transaction:
		return s.processTransaction(t)
	case *blockchain.Block:
		return s.processBlock(t)
	case *GetStatusMessage:
		return s.processGetStatusMessage(msg.From, t)
	case *StatusMessage:
		return s.processStatusMessage(msg.From, t)
	case *GetBlocksMessage:
		return s.processGetBlocksMessage(msg.From, t)
	case *BlocksMessage:
		return s.processBlocksMessage(msg.From, t)
	default:
		return nil
	}
}

func (s *Server) processGetBlocksMessage(from net.Addr, data *GetBlocksMessage) error {
	fmt.Printf("[SMOOTH-LOG] Received getBlocks request from %s\n", from)

	var blocks []*blockchain.Block
	ourHeight := s.chain.Height()

	if data.To == 0 {
		for i := int(data.From); i <= int(ourHeight); i++ {
			block, err := s.chain.GetBlock(uint32(i))
			if err != nil {
				return err
			}
			blocks = append(blocks, block)
		}
	}

	blocksMsg := &BlocksMessage{Blocks: blocks}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(blocksMsg); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	msg := NewMessage(MessageTypeBlocks, buf.Bytes())
	peer, ok := s.peerMap[from]
	if !ok {
		return fmt.Errorf("peer %s not known", from)
	}
	return peer.Send(msg.Bytes())
}

func (s *Server) sendGetStatusMessage(peer *TCPPeer) error {
	getStatusMsg := new(GetStatusMessage)
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(getStatusMsg); err != nil {
		return err
	}
	msg := NewMessage(MessageTypeGetStatus, buf.Bytes())
	return peer.Send(msg.Bytes())
}

func (s *Server) broadcast(payload []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for netAddr, peer := range s.peerMap {
		if err := peer.Send(payload); err != nil {
			fmt.Printf("[SMOOTH-LOG] Error sending to %s: %v\n", netAddr, err)
		}
	}
	return nil
}

func (s *Server) processBlocksMessage(from net.Addr, data *BlocksMessage) error {
	fmt.Printf("[SMOOTH-LOG] Received BLOCKS from %s\n", from)
	for _, block := range data.Blocks {
		if err := s.chain.AddBlock(block); err != nil {
			fmt.Printf("[SMOOTH-LOG] Error adding block: %v\n", err)
			return err
		}
	}
	return nil
}

func (s *Server) processStatusMessage(from net.Addr, data *StatusMessage) error {
	fmt.Printf("[SMOOTH-LOG] Received status from %s (height=%d)\n", from, data.CurrentHeight)
	if data.CurrentHeight <= s.chain.Height() {
		fmt.Printf("[SMOOTH-LOG] Not syncing — ourHeight=%d, theirHeight=%d\n", s.chain.Height(), data.CurrentHeight)
		return nil
	}
	go s.requestBlocksLoop(from)
	return nil
}

func (s *Server) processGetStatusMessage(from net.Addr, data *GetStatusMessage) error {
	fmt.Printf("[SMOOTH-LOG] Got status request from %s\n", from)

	statusMessage := &StatusMessage{
		CurrentHeight: s.chain.Height(),
		ID:            s.ID,
	}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(statusMessage); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, ok := s.peerMap[from]
	if !ok {
		return fmt.Errorf("peer %s not known", from)
	}
	msg := NewMessage(MessageTypeStatus, buf.Bytes())
	return peer.Send(msg.Bytes())
}

func (s *Server) processBlock(b *blockchain.Block) error {
	if err := s.chain.AddBlock(b); err != nil {
		fmt.Printf("[SMOOTH-LOG] Error adding block: %v\n", err)
		return err
	}
	go s.broadcastBlock(b)
	return nil
}

func (s *Server) processTransaction(tx *blockchain.Transaction) error {
	hash := tx.Hash(blockchain.TxHasher{})
	if s.mempool.Contains(hash) {
		return nil
	}
	if err := tx.Verify(); err != nil {
		return err
	}
	go s.broadcastTx(tx)
	s.mempool.Add(tx)
	return nil
}

func (s *Server) requestBlocksLoop(addr net.Addr) error {
	ticker := time.NewTicker(3 * time.Second)
	for {
		ourHeight := s.chain.Height()
		fmt.Printf("[SMOOTH-LOG] Requesting blocks from height %d\n", ourHeight+1)

		getBlocksMessage := &GetBlocksMessage{
			From: ourHeight + 1,
			To:   0,
		}
		buf := new(bytes.Buffer)
		if err := gob.NewEncoder(buf).Encode(getBlocksMessage); err != nil {
			return err
		}

		s.mu.RLock()
		defer s.mu.RUnlock()

		peer, ok := s.peerMap[addr]
		if !ok {
			return fmt.Errorf("peer %s not known", addr)
		}
		if err := peer.Send(NewMessage(MessageTypeGetBlocks, buf.Bytes()).Bytes()); err != nil {
			fmt.Printf("[SMOOTH-LOG] Error requesting blocks: %v\n", err)
		}

		<-ticker.C
	}
}

func (s *Server) broadcastBlock(b *blockchain.Block) error {
	buf := &bytes.Buffer{}
	if err := b.Encode(blockchain.NewGobBlockEncoder(buf)); err != nil {
		return err
	}
	msg := NewMessage(MessageTypeBlock, buf.Bytes())
	return s.broadcast(msg.Bytes())
}

func (s *Server) broadcastTx(tx *blockchain.Transaction) error {
	buf := &bytes.Buffer{}
	if err := tx.Encode(blockchain.NewGobTxEncoder(buf)); err != nil {
		return err
	}
	msg := NewMessage(MessageTypeTx, buf.Bytes())
	return s.broadcast(msg.Bytes())
}

func (s *Server) createNewBlock() error {
	currentHeader, err := s.chain.GetHeader(s.chain.Height())
	if err != nil {
		return err
	}
	txx := s.mempool.Pending()
	block, err := blockchain.NewBlockFromPrevHeader(currentHeader, txx)
	if err != nil {
		return err
	}
	if err := block.Sign(*s.PrivateKey); err != nil {
		return err
	}
	if err := s.chain.AddBlock(block); err != nil {
		return err
	}
	s.mempool.ClearPending()
	go s.broadcastBlock(block)
	return nil
}

func genesisBlock() *blockchain.Block {
	header := &blockchain.Header{
		Version:   1,
		DataHash:  types.Hash{},
		Height:    0,
		Timestamp: 0,
	}
	b, _ := blockchain.NewBlock(header, nil)
	coinbase := crypto_utils.PublicKey{}
	tx := blockchain.NewTransaction(nil)
	tx.From = coinbase
	tx.To = coinbase
	tx.Value = 10_000_000
	b.Transactions = append(b.Transactions, tx)

	privKey := crypto_utils.GeneratePrivateKey()
	if err := b.Sign(privKey); err != nil {
		panic(err)
	}

	return b
}
