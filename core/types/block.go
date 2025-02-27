// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package types contains data types related to Ethereum consensus.
package types

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/common/hexutil"
	"github.com/clearmatics/autonity/crypto"
	"github.com/clearmatics/autonity/rlp"
	"golang.org/x/crypto/sha3"
)

var (
	EmptyRootHash  = common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	EmptyUncleHash = rlpHash([]*Header(nil))
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the Autonity blockchain.
type Header struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        uint64         `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"`
	Nonce       BlockNonce     `json:"nonce"`

	/*
		PoS header fields, round & committedSeals not taken into account
		for computing the sigHash.
	*/
	Committee Committee `json:"committee"           gencodec:"required"`
	// used for committee member lookup, lazily initialised.
	committeeMap map[common.Address]*CommitteeMember
	// Used to ensure the committeeMap is created only once.
	once sync.Once

	ProposerSeal       []byte   `json:"proposerSeal"        gencodec:"required"`
	Round              uint64   `json:"round"               gencodec:"required"`
	CommittedSeals     [][]byte `json:"committedSeals"      gencodec:"required"`
	PastCommittedSeals [][]byte `json:"pastCommittedSeals"  gencodec:"required"`
}

type CommitteeMember struct {
	Address     common.Address `json:"address"            gencodec:"required"       abi:"addr"`
	VotingPower *big.Int       `json:"votingPower"        gencodec:"required"`
}

// MarshalText encodes b as a hex string with 0x prefix.
func (c *CommitteeMember) MarshalText() ([]byte, error) {
	data, err := rlp.EncodeToBytes(c)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(data).MarshalText()
}

// UnmarshalText b as a hex string with 0x prefix.
func (c *CommitteeMember) UnmarshalText(input []byte) error {
	var b hexutil.Bytes
	err := b.UnmarshalText(input)
	if err != nil {
		return err
	}
	return rlp.DecodeBytes(b, &c)
}

type Committee []CommitteeMember

// originalHeader represents the ethereum blockchain header.
type originalHeader struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        uint64         `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"`
	Nonce       BlockNonce     `json:"nonce"`
}

type headerExtra struct {
	Committee          Committee `json:"committee"           gencodec:"required"`
	ProposerSeal       []byte    `json:"proposerSeal"        gencodec:"required"`
	Round              uint64    `json:"round"               gencodec:"required"`
	CommittedSeals     [][]byte  `json:"committedSeals"      gencodec:"required"`
	PastCommittedSeals [][]byte  `json:"pastCommittedSeals"  gencodec:"required"`
}

// headerMarshaling is used by gencodec (which can be invoked bu running go
// generate in this package) and defines marshalling types for fields that
// would not marshal correctly to hex of their own accord. When modifying the
// structure of Header, this will likely need to be updated before running go
// generate to regenerate the json marshalling code.
type headerMarshaling struct {
	Difficulty *hexutil.Big
	Number     *hexutil.Big
	GasLimit   hexutil.Uint64
	GasUsed    hexutil.Uint64
	Time       hexutil.Uint64
	Extra      hexutil.Bytes
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
	/*
		PoS header fields type overriedes
	*/
	ProposerSeal       hexutil.Bytes
	Round              hexutil.Uint64
	CommittedSeals     []hexutil.Bytes
	PastCommittedSeals []hexutil.Bytes
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	// If the mix digest is equivalent to the predefined BFT digest, use BFT
	// specific hash calculation. This is always the case with tendermint consensus protocol.
	if h.MixDigest == BFTDigest {
		// Seal is reserved in extra-data. To prove block is signed by the proposer.
		if posHeader := BFTFilteredHeader(h, true); posHeader != nil {
			return rlpHash(posHeader)
		}
	}

	// If not using the BFT mixdigest then return the original ethereum block header hash, this
	// let Autonity to remain compatible with original go-ethereum tests.
	return rlpHash(h.original())
}

var headerSize = common.StorageSize(reflect.TypeOf(Header{}).Size())

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return headerSize + common.StorageSize(len(h.Extra)+(h.Difficulty.BitLen()+h.Number.BitLen())/8)
}

// SanityCheck checks a few basic things -- these checks are way beyond what
// any 'sane' production values should hold, and can mainly be used to prevent
// that the unbounded fields are stuffed with junk data to add processing
// overhead
func (h *Header) SanityCheck() error {
	if h.Number != nil && !h.Number.IsUint64() {
		return fmt.Errorf("too large block number: bitlen %d", h.Number.BitLen())
	}
	if h.Difficulty != nil {
		if diffLen := h.Difficulty.BitLen(); diffLen > 80 {
			return fmt.Errorf("too large block difficulty: bitlen %d", diffLen)
		}
	}
	if eLen := len(h.Extra); eLen > 100*1024 {
		return fmt.Errorf("too large block extradata: size %d", eLen)
	}
	return nil
}

// DecodeRLP decodes the Ethereum
func (h *Header) DecodeRLP(s *rlp.Stream) error {
	origin := &originalHeader{}
	if err := s.Decode(origin); err != nil {
		return err
	}

	hExtra := &headerExtra{}
	if origin.MixDigest == BFTDigest {
		err := rlp.DecodeBytes(origin.Extra, hExtra)
		if err != nil {
			return err
		}
		h.CommittedSeals = hExtra.CommittedSeals
		h.Committee = hExtra.Committee
		h.PastCommittedSeals = hExtra.PastCommittedSeals
		h.ProposerSeal = hExtra.ProposerSeal
		h.Round = hExtra.Round
	} else {
		h.Extra = origin.Extra
	}

	h.ParentHash = origin.ParentHash
	h.UncleHash = origin.UncleHash
	h.Coinbase = origin.Coinbase
	h.Root = origin.Root
	h.TxHash = origin.TxHash
	h.ReceiptHash = origin.ReceiptHash
	h.Bloom = origin.Bloom
	h.Difficulty = origin.Difficulty
	h.Number = origin.Number
	h.GasLimit = origin.GasLimit
	h.GasUsed = origin.GasUsed
	h.Time = origin.Time
	h.MixDigest = origin.MixDigest
	h.Nonce = origin.Nonce

	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
//
// To maintain RLP compatibility with eth tooling we have to encode our
// additional header fields into the extra data field. RLP decoding expects the
// encoded data to have an exact number of fields of a certain type in a
// particular order, if there is a mismatch decoding fails. So to maintain
// compatibility with ethereum we encode all our additional header fields into
// the extra data field leaving us with just the original ethereum header
// fields. When we decode we repopulate our additional header fields from the
// extra data.
func (h *Header) EncodeRLP(w io.Writer) error {
	hExtra := headerExtra{
		Committee:          h.Committee,
		ProposerSeal:       h.ProposerSeal,
		Round:              h.Round,
		CommittedSeals:     h.CommittedSeals,
		PastCommittedSeals: h.PastCommittedSeals,
	}

	original := h.original()
	if h.MixDigest == BFTDigest {
		extra, err := rlp.EncodeToBytes(hExtra)
		if err != nil {
			return err
		}
		original.Extra = extra
	} else {
		original.Extra = h.Extra
	}

	return rlp.Encode(w, *original)
}

func (h *Header) original() *originalHeader {
	return &originalHeader{
		ParentHash:  h.ParentHash,
		UncleHash:   h.UncleHash,
		Coinbase:    h.Coinbase,
		Root:        h.Root,
		TxHash:      h.TxHash,
		ReceiptHash: h.ReceiptHash,
		Bloom:       h.Bloom,
		Difficulty:  h.Difficulty,
		Number:      h.Number,
		GasLimit:    h.GasLimit,
		GasUsed:     h.GasUsed,
		Time:        h.Time,
		Extra:       h.Extra,
		MixDigest:   h.MixDigest,
		Nonce:       h.Nonce,
	}
}

// hasherPool holds LegacyKeccak hashers.
var hasherPool = sync.Pool{
	New: func() interface{} {
		return sha3.NewLegacyKeccak256()
	},
}

func rlpHash(x interface{}) (h common.Hash) {
	sha := hasherPool.Get().(crypto.KeccakState)
	defer hasherPool.Put(sha)
	sha.Reset()
	rlp.Encode(sha, x)
	sha.Read(h[:])
	return h
}

// EmptyBody returns true if there is no additional 'body' to complete the header
// that is: no transactions and no uncles.
func (h *Header) EmptyBody() bool {
	return h.TxHash == EmptyRootHash && h.UncleHash == EmptyUncleHash
}

// EmptyReceipts returns true if there are no receipts for this header/block.
func (h *Header) EmptyReceipts() bool {
	return h.ReceiptHash == EmptyRootHash
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type Body struct {
	Transactions []*Transaction
	Uncles       []*Header
}

// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	header       *Header
	uncles       []*Header
	transactions Transactions

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *Block) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
type StorageBlock Block

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.
type storageblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
	TD     *big.Int
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt, hasher Hasher) *Block {
	b := &Block{header: CopyHeader(header), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs), hasher)
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts), hasher)
		b.header.Bloom = CreateBloom(receipts)
	}

	if len(uncles) == 0 {
		b.header.UncleHash = EmptyUncleHash
	} else {
		b.header.UncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i])
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {

	difficulty := big.NewInt(0)
	if h.Difficulty != nil {
		difficulty.Set(h.Difficulty)
	}

	number := big.NewInt(0)
	if h.Number != nil {
		number.Set(h.Number)
	}

	extra := make([]byte, 0)
	if len(h.Extra) > 0 {
		extra = make([]byte, len(h.Extra))
		copy(extra, h.Extra)
	}

	/* PoS fields deep copy section*/
	committee := make([]CommitteeMember, 0)
	if len(h.Committee) > 0 {
		committee = make([]CommitteeMember, len(h.Committee))
		for i, val := range h.Committee {
			committee[i] = CommitteeMember{
				Address:     val.Address,
				VotingPower: new(big.Int).Set(val.VotingPower),
			}
		}
	}

	proposerSeal := make([]byte, 0)
	if len(h.ProposerSeal) > 0 {
		proposerSeal = make([]byte, len(h.ProposerSeal))
		copy(proposerSeal, h.ProposerSeal)
	}

	committedSeals := make([][]byte, 0)
	if len(h.CommittedSeals) > 0 {
		committedSeals = make([][]byte, len(h.CommittedSeals))
		for i, val := range h.CommittedSeals {
			committedSeals[i] = make([]byte, len(val))
			copy(committedSeals[i], val)
		}
	}

	pastCommittedSeals := make([][]byte, 0)
	if len(h.PastCommittedSeals) > 0 {
		pastCommittedSeals = make([][]byte, len(h.PastCommittedSeals))
		for i, val := range h.PastCommittedSeals {
			pastCommittedSeals[i] = make([]byte, len(val))
			copy(pastCommittedSeals[i], val)
		}
	}

	cpy := &Header{
		ParentHash:         h.ParentHash,
		UncleHash:          h.UncleHash,
		Coinbase:           h.Coinbase,
		Root:               h.Root,
		TxHash:             h.TxHash,
		ReceiptHash:        h.ReceiptHash,
		Bloom:              h.Bloom,
		Difficulty:         difficulty,
		Number:             number,
		GasLimit:           h.GasLimit,
		GasUsed:            h.GasUsed,
		Time:               h.Time,
		Extra:              extra,
		MixDigest:          h.MixDigest,
		Nonce:              h.Nonce,
		Committee:          committee,
		ProposerSeal:       proposerSeal,
		Round:              h.Round,
		CommittedSeals:     committedSeals,
		PastCommittedSeals: pastCommittedSeals,
	}
	return cpy
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions = eb.Header, eb.Uncles, eb.Txs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
		Uncles: b.uncles,
	})
}

// [deprecated by eth/63]
func (b *StorageBlock) DecodeRLP(s *rlp.Stream) error {
	var sb storageblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions, b.td = sb.Header, sb.Uncles, sb.Txs, sb.TD
	return nil
}

// TODO: copies

func (b *Block) Uncles() []*Header          { return b.uncles }
func (b *Block) Transactions() Transactions { return b.transactions }

func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}

func (b *Block) Number() *big.Int     { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasLimit() uint64     { return b.header.GasLimit }
func (b *Block) GasUsed() uint64      { return b.header.GasUsed }
func (b *Block) Difficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *Block) Time() uint64         { return b.header.Time }

func (b *Block) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *Block) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *Block) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *Block) Bloom() Bloom             { return b.header.Bloom }
func (b *Block) Coinbase() common.Address { return b.header.Coinbase }
func (b *Block) Root() common.Hash        { return b.header.Root }
func (b *Block) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash      { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *Block) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *Block) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *Block) Header() *Header { return CopyHeader(b.header) }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions, b.uncles} }

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *Block) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// SanityCheck can be used to prevent that unbounded fields are
// stuffed with junk data to add processing overhead
func (b *Block) SanityCheck() error {
	return b.header.SanityCheck()
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

func CalcUncleHash(uncles []*Header) common.Hash {
	if len(uncles) == 0 {
		return EmptyUncleHash
	}
	// len(uncles) > 0 can only happen during tests.
	// We revert to the original structure to keep compatibility with hardcoded hash values.
	originalUncles := make([]*originalHeader, len(uncles))
	for i := range uncles {
		originalUncles[i] = uncles[i].original()
	}
	return rlpHash(originalUncles)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		header:       &cpy,
		transactions: b.transactions,
		uncles:       b.uncles,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
func (b *Block) WithBody(transactions []*Transaction, uncles []*Header) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
		uncles:       make([]*Header, len(uncles)),
	}
	copy(block.transactions, transactions)
	for i := range uncles {
		block.uncles[i] = CopyHeader(uncles[i])
	}
	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

type Blocks []*Block
