/*
Package twopc provides a simple but complete library for two-phase commit
protocol for transcations.

Firstly, we should setup the static topology for two-phase commit, including
one Coordinator and serveral Participants. Participant.RegisterCaller could
bind user-defined Caller to an identifier.

The transcation has an initial logic and the subsequent logic, which is made
up of several parts (named as TxnPart), each of which is executed on a
specific Participant.

The initial logic decides whether the txn will continue or not. Only the txn
continues, the TxnParts will be submitted to Participants and the executed.
Every TxnPart has a Caller, which finally gives a errCode. ErrCode reveals
whether the part of transaction fails or succeeds. If one of them fails, all
the TxnPart should roll back the state, which is the essense of two-phase
commit protocol.

The error code finally given to the user is the result of OR operation among
the error codes of all the TxnPart. So the error code of every TxnPart should
be 0 (success) or more than 0. A user can define the error codes for various
situations.

Coordinator and Participant are the roles of two-phase commit protocol.
CommitTxn and AbortTxn are mutually exclusive, which is the essence of
two-phase commit protocol. So please notice AbortTxn because of timeout and
simultaneous CommitTxn. Also, the two functions must be invoked at most once.

Fault-tolerance model

We assume that at least one participant is OK. So timeout could be started to
calculated from receiving the 1st StatePrepared.

We assume that there is no fault in the Coordinator, including the
communication errors (timeout, socket close, etc), the process and power
failures.

State of part of transaction

Initial state is StateTxnPartWorking when Participant receives TxnPart
submitted by the Coordinator. StateTxnPartWorking could be transferred to
StateTxnPartPrepared if TxnPart is processed successfully. Otherwise,
StateTxnPartWorking is transferred to StateTxnPartAborted. Only if the
Coordinator informs the Participant that the corresponding Txn could commit,
i.e. receiving all the Prepared msgs, the TxnPart could be changed to
StateTxnPartCommitted.

State of transaction

Initial state is StateTxnCreated. StateTxnCreated changes to StateTxnInit
only if the initial function of Txn is successfully executed. StateTxnInit
could be transferred to StateTxnPreparing if 1st received msg is Prepared; it
could be transferred to StateTxnAborted if 1st received msg is Aborted.
StateTxnPreparing could be transferred to StateTxnAborted if receving any
Aborted msg or timeout after receiving the 1st Prepared msg. StateTxnPrepared
could be transferred to StateTxnCommitted if receving all Prepared msgs.

Reference

Consensus on Transaction Commit. https://www.microsoft.com/en-us/research/publication/consensus-on-transaction-commit/.
*/
package twopc

import (
	"distributed-system/util"
	"fmt"
	"log"
	"math"
	"net"
	"net/rpc"
	"sync"
	"sync/atomic"
	"time"
)

// BUG():.

// TODO. Slot distributing for parallel txns.

// Coordinator is the manager role of two-phase commit.
type Coordinator struct {
	mu   sync.Mutex
	l    net.Listener
	rpcs *rpc.Server
	pa   *util.ResourcePoolsArray

	coord string   // coordinator address
	ppts  []string // participants addresses
	slots []bool   // participants slots

	network string
	txnsMu  sync.RWMutex
	txns    map[string]*Txn
}

// CoordClientMaxSizeForOnePpt is the maximum number of connections in the pool
// from the Coordinator to a Participant.
const CoordClientMaxSizeForOnePpt = 100

// NewCoordinator init a Coordinator service. Network could be "tcp" or "unix".
// Coord is the listened address on the Coordiantor. Ppts is
// the listened addresses of the list of all the Participants.
func NewCoordinator(network, coord string, ppts []string) *Coordinator {
	ctr := &Coordinator{network: network, coord: coord, ppts: ppts,
		txns: make(map[string]*Txn)}
	news := make([]func() util.Resource, len(ppts))
	ctr.slots = make([]bool, len(ppts))
	for i := 0; i < len(ppts); i++ {
		addr := ppts[i]
		news[i] = func() util.Resource {
			return util.DialServer(network, addr)
		}
	}
	ctr.pa = util.NewResourcePoolsArray(news,
		CoordClientMaxSizeForOnePpt, len(ppts))

	// Don't change any of the following code,
	// or do anything to subvert it.

	// Create a thread to accept RPC connections
	l, e := net.Listen(network, coord)
	if e != nil {
		log.Printf("listen error: %v\n", e)
	} else {
		log.Printf("listen successfully @%v\n", coord)
	}
	ctr.l = l
	rpcs := rpc.NewServer()
	rpcs.Register(ctr)
	ctr.rpcs = rpcs

	// Don't change any of the following code,
	// or do anything to subvert it.

	// Create a thread to accept RPC connections
	go func() {
		for {
			conn, err := ctr.l.Accept()
			if err == nil {
				go rpcs.ServeConn(conn)
			} else {
				fmt.Printf("Coordinator accept: %v\n", err.Error())
			}
		}
	}()
	return ctr
}

// RegisterService registers the rpc calls of the service onto the
// Coordinator.
func (ctr *Coordinator) RegisterService(service interface{}) {
	ctr.rpcs.Register(service)
}

// NewTxn initialize a new Txn. The user-defined initial txn function, hash
// function for key, and the timeout should be set. The coordinator assign
// an unique ID for the transaction.
//
// It's thread-safe.
func (ctr *Coordinator) NewTxn(initFunc TxnInitFunc,
	keyHashFunc KeyHashFunc, timeoutMs int64) *Txn {
	// TODO
	txn := &Txn{ID: nrand(), ctr: ctr,
		partsNum: 0, parts: nil, preparedCnt: 0,
		done: make(chan struct{}, 1), state: StateTxnCreated,
		timeoutMs: timeoutMs, initFunc: initFunc,
		keyHashFunc: keyHashFunc,
	}
	ctr.txnsMu.Lock()
	defer ctr.txnsMu.Unlock()
	ctr.txns[txn.ID] = txn
	return txn
}

// TxnState is the state of the transaction.
type TxnState struct {
	State   int32
	ErrCode int
}

// StateTxn is a RPC call that returns the latest state of the transcation.
func (ctr *Coordinator) StateTxn(txnID *string, reply *TxnState) error {
	txn := ctr.txnByID(*txnID)
	reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	return nil
}

// SyncTxnEnd is a RPC call wait until the state of the transaction changed
// to StateTxnAborted or StateTxnCommitted.
func (ctr *Coordinator) SyncTxnEnd(txnID *string, reply *TxnState) error {
	txn := ctr.txnByID(*txnID)
	reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	retryCnt := 0
	for reply.State != StateTxnAborted && reply.State != StateTxnCommitted {
		retryCnt++
		waitTime := int64(math.Pow(2, float64(retryCnt)))
		time.Sleep(time.Millisecond * time.Duration(waitTime))
		reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	}
	return nil
}

// Abort triggers when users want to actively abort the
// transaction in some conditions.
func (ctr *Coordinator) Abort(txnID string) {
	txn := ctr.txnByID(txnID)
	txn.abortTxnPart(-1, 0)
	txn.errCode = ErrTxnUserAbort
}

// ------------ RPC calls START -------------

// InformPrepared is a RPC call invoked by the participant when it informs
// the Coordinator of the prepared state.
func (ctr *Coordinator) InformPrepared(args *PreparedArgs, reply *PreparedReply) error {
	// TODO
	txn := ctr.txnByID(args.TxnID)
	txn.prepareTxnPart(args.TxnPartIdx, args.ErrCode)

	// Only the 1st prepared will trigger the WaitAllPrepared().
	swapped := atomic.CompareAndSwapInt32(&txn.state, StateTxnInit, StateTxnPreparing)
	if swapped {
		go txn.waitAllPartsPrepared()
	}
	return nil
}

// InformAborted is a RPC call invoked by the participant when it informs
// the Coordinator of the aborted state.
func (ctr *Coordinator) InformAborted(args *AbortedArgs, reply *AbortedReply) error {
	// TODO
	txn := ctr.txnByID(args.TxnID)
	txn.abortTxnPart(args.TxnPartIdx, args.ErrCode)
	return nil
}

// ------------ RPC calls END -------------

func (ctr *Coordinator) txnByID(txnID string) *Txn {
	ctr.txnsMu.RLock()
	defer ctr.txnsMu.RUnlock()
	txn := ctr.txns[txnID]
	return txn
}

// Please do not change the following methods.

// Kill tell the coordinator to shut itself down for testing.
func (ctr *Coordinator) Kill() {
	if ctr.l != nil {
		ctr.l.Close()
	}
}
