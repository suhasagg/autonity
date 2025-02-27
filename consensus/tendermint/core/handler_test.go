package core

import (
	"context"
	"github.com/influxdata/influxdb/pkg/deep"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/consensus/tendermint/config"
	"github.com/clearmatics/autonity/consensus/tendermint/events"
	"github.com/clearmatics/autonity/crypto"
	"github.com/clearmatics/autonity/event"
	"github.com/clearmatics/autonity/log"
	"github.com/clearmatics/autonity/rlp"
	"github.com/golang/mock/gomock"
)

func TestHandleCheckedMessage(t *testing.T) {
	committeeSet, keysMap := newTestCommitteeSetWithKeys(4)
	currentValidator, _ := committeeSet.GetByIndex(0)
	sender, _ := committeeSet.GetByIndex(1)
	senderKey := keysMap[sender.Address]

	createPrevote := func(round int64, height int64) *Message {
		vote := &Vote{
			Round:             round,
			Height:            big.NewInt(height),
			ProposedBlockHash: common.BytesToHash([]byte{0x1}),
		}
		encoded, err := rlp.EncodeToBytes(&vote)
		if err != nil {
			t.Fatalf("could not encode vote")
		}
		return &Message{
			Code:    msgPrevote,
			Msg:     encoded,
			Address: sender.Address,
		}
	}

	createPrecommit := func(round int64, height int64) *Message {
		vote := &Vote{
			Round:             round,
			Height:            big.NewInt(height),
			ProposedBlockHash: common.BytesToHash([]byte{0x1}),
		}
		encoded, err := rlp.EncodeToBytes(&vote)
		if err != nil {
			t.Fatalf("could not encode vote")
		}
		data := PrepareCommittedSeal(common.BytesToHash([]byte{0x1}), vote.Round, vote.Height)
		hashData := crypto.Keccak256(data)
		commitSign, err := crypto.Sign(hashData, senderKey)
		if err != nil {
			t.Fatalf("error signing")
		}
		return &Message{
			Code:          msgPrecommit,
			Msg:           encoded,
			Address:       sender.Address,
			CommittedSeal: commitSign,
		}
	}

	cases := []struct {
		round   int64
		height  *big.Int
		step    Step
		message *Message
		outcome error
		panic   bool
	}{
		{
			1,
			big.NewInt(2),
			propose,
			createPrevote(1, 2),
			errFutureStepMessage,
			false,
		},
		{
			1,
			big.NewInt(2),
			propose,
			createPrevote(2, 2),
			errFutureRoundMessage,
			false,
		},
		{
			0,
			big.NewInt(2),
			propose,
			createPrevote(0, 3),
			errFutureHeightMessage,
			true,
		},
		{
			0,
			big.NewInt(2),
			prevote,
			createPrevote(0, 2),
			nil,
			false,
		},
		{
			0,
			big.NewInt(2),
			precommit,
			createPrecommit(0, 2),
			nil,
			false,
		},
		{
			0,
			big.NewInt(5),
			precommit,
			createPrecommit(0, 10),
			errFutureHeightMessage,
			true,
		},
		{
			5,
			big.NewInt(2),
			precommit,
			createPrecommit(20, 2),
			errFutureRoundMessage,
			false,
		},
	}

	for _, testCase := range cases {
		logger := log.New("backend", "test", "id", 0)
		message := newMessagesMap()
		engine := core{
			logger:            logger,
			address:           currentValidator.Address,
			backlogs:          make(map[common.Address][]*Message),
			round:             testCase.round,
			height:            testCase.height,
			step:              testCase.step,
			futureRoundChange: make(map[int64]map[common.Address]uint64),
			messages:          message,
			curRoundMessages:  message.getOrCreate(0),
			committee:         committeeSet,
			proposeTimeout:    newTimeout(propose, logger),
			prevoteTimeout:    newTimeout(prevote, logger),
			precommitTimeout:  newTimeout(precommit, logger),
		}

		func() {
			defer func() {
				r := recover()
				if r == nil && testCase.panic {
					t.Errorf("The code did not panic")
				}
				if r != nil && !testCase.panic {
					t.Errorf("Unexpected panic")
				}
			}()

			err := engine.handleCheckedMsg(context.Background(), testCase.message)

			if err != testCase.outcome {
				t.Fatal("unexpected handlecheckedmsg returning ",
					"err=", err, ", expecting=", testCase.outcome, " with msgCode=", testCase.message.Code)
			}

			if err != nil {
				backlogValue := engine.backlogs[sender.Address][0]
				if backlogValue != testCase.message {
					t.Fatal("unexpected backlog message")
				}
			}
		}()
	}
}

func TestHandleMsg(t *testing.T) {
	t.Run("old height message return error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		backendMock := NewMockBackend(ctrl)
		c := &core{
			logger:   log.New("backend", "test", "id", 0),
			backend:  backendMock,
			address:  common.HexToAddress("0x1234567890"),
			backlogs: make(map[common.Address][]*Message),
			step:     propose,
			round:    1,
			height:   big.NewInt(2),
		}
		vote := &Vote{
			Round:             2,
			Height:            big.NewInt(1),
			ProposedBlockHash: common.BytesToHash([]byte{0x1}),
		}
		payload, err := rlp.EncodeToBytes(vote)
		require.NoError(t, err)
		msg := &Message{
			Code:       msgPrevote,
			Msg:        payload,
			decodedMsg: vote,
			Address:    common.Address{},
		}

		if err := c.handleMsg(context.Background(), msg); err != errOldHeightMessage {
			t.Fatal("errOldHeightMessage not returned")
		}
	})

	t.Run("future height message return error but are saved", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		backendMock := NewMockBackend(ctrl)
		c := &core{
			logger:           log.New("backend", "test", "id", 0),
			backend:          backendMock,
			address:          common.HexToAddress("0x1234567890"),
			backlogs:         make(map[common.Address][]*Message),
			backlogUnchecked: map[uint64][]*Message{},
			step:             propose,
			round:            1,
			height:           big.NewInt(2),
		}
		vote := &Vote{
			Round:             2,
			Height:            big.NewInt(3),
			ProposedBlockHash: common.BytesToHash([]byte{0x1}),
		}
		payload, err := rlp.EncodeToBytes(vote)
		require.NoError(t, err)
		msg := &Message{
			Code:       msgPrevote,
			Msg:        payload,
			decodedMsg: vote,
			Address:    common.Address{},
		}

		if err := c.handleMsg(context.Background(), msg); err != errFutureHeightMessage {
			t.Fatal("errFutureHeightMessage not returned")
		}
		if backlog, ok := c.backlogUnchecked[3]; !(ok && len(backlog) > 0 && deep.Equal(backlog[0], msg)) {
			t.Fatal("future message not saved in the untrusted buffer")
		}
	})
}

func TestCoreStopDoesntPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	addr := common.HexToAddress("0x0123456789")

	backendMock := NewMockBackend(ctrl)
	backendMock.EXPECT().Address().AnyTimes().Return(addr)

	logger := log.New("testAddress", "0x0000")
	eMux := event.NewTypeMuxSilent(logger)
	sub := eMux.Subscribe(events.MessageEvent{})

	backendMock.EXPECT().Subscribe(gomock.Any()).Return(sub).MaxTimes(5)

	c := New(backendMock, config.DefaultConfig())
	_, c.cancel = context.WithCancel(context.Background())
	c.subscribeEvents()
	c.stopped <- struct{}{}
	c.stopped <- struct{}{}
	c.stopped <- struct{}{}

	c.Stop()
}
