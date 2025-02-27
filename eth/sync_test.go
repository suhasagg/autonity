// Copyright 2015 The go-ethereum Authors
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

package eth

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/clearmatics/autonity/eth/downloader"
	"github.com/clearmatics/autonity/p2p"
)

func TestFastSyncDisabling63(t *testing.T) { testFastSyncDisabling(t, 63) }
func TestFastSyncDisabling64(t *testing.T) { testFastSyncDisabling(t, 64) }
func TestFastSyncDisabling65(t *testing.T) { testFastSyncDisabling(t, 65) }

// Tests that fast sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func testFastSyncDisabling(t *testing.T, protocol int) {
	// These tests rely on 2 peers not sycing with each other, so we set the
	// min sync peers to be 2 and then restore the default later.
	originalDefaultMinSyncPeers := defaultMinSyncPeers
	defaultMinSyncPeers = 2
	defer func() { defaultMinSyncPeers = originalDefaultMinSyncPeers }()

	//t.Parallel() (Parrallelisation is unsafe due to the enode package)
	emptyNode := newTestP2PPeer("empty")
	fullNode := newTestP2PPeer("full")
	enodes := []string{emptyNode.Info().Enode, fullNode.Info().Enode}
	// Create a pristine protocol manager, check that fast sync is left enabled
	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FastSync, 0, nil, nil, enodes)
	if atomic.LoadUint32(&pmEmpty.fastSync) == 0 {
		t.Fatalf("fast sync disabled on pristine blockchain")
	}
	// Create a full protocol manager, check that fast sync gets disabled
	pmFull, _ := newTestProtocolManagerMust(t, downloader.FastSync, 1024, nil, nil, enodes)
	if atomic.LoadUint32(&pmFull.fastSync) == 1 {
		t.Fatalf("fast sync not disabled on non-empty blockchain")
	}

	// Sync up the two peers
	io1, io2 := p2p.MsgPipe()
	go pmFull.handle(pmFull.newPeer(protocol, emptyNode, io2, pmFull.txpool.Get))
	go pmEmpty.handle(pmEmpty.newPeer(protocol, fullNode, io1, pmEmpty.txpool.Get))

	time.Sleep(250 * time.Millisecond)
	op := peerToSyncOp(downloader.FastSync, pmEmpty.peers.BestPeer())
	if err := pmEmpty.doSync(op); err != nil {
		t.Fatal("sync failed:", err)
	}

	// Check that fast sync was disabled
	if atomic.LoadUint32(&pmEmpty.fastSync) == 1 {
		t.Fatalf("fast sync not disabled after successful synchronisation")
	}
}
