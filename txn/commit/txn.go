package commit

// Assume the coordinator hasn't any failures.
//
// Reference:
// * Consensus on Transaction Commit

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Status of Txn
const (
	StateTxnCreated = iota
	StateTxnInit
	StateTxnPreparing
	StateTxnCommitted
	StateTxnAborted
)

// Status of TxnPart
const (
	StateTxnPartWorking = iota
	StateTxnPartPrepared
	StateTxnPartCommitted
	StateTxnPartAborted
)

// TxnPart reply
const (
	ErrTimeout   = -1
	ErrUserAbort = -2
)

// Caller is the execution logic.
// Success when errCode is 0, otherwise failed, which means
// that the TxnPart aborts.
// @args is the user args. It could be nil.
// @initRet is the return value of initFunction of Txn. It could be nil.
// @errCode is the error code of TxnPart, must be more than 0.
// @rb is the rollback of the TxnPart.
type Caller interface {
	Call(args interface{}, initRet interface{}) (errCode int, rb Rollbacker)
}

type CallFunc func(args interface{}, initRet interface{}) (errCode int, rb Rollbacker)

func (f CallFunc) Call(args interface{}, initRet interface{}) (errCode int, rb Rollbacker) {
	errCode, rb = f(args, initRet)
	return
}

type RollbackFunc func()

var BlankRollbackFunc RollbackFunc = func() {}

type Rollbacker interface {
	Rollback()
}

func (f RollbackFunc) Rollback() {
	f()
}

// TxnInitFunc is the initialization before Txn processing.
// The returning errCode indicates the state of the procedure,
// which decides whether the following Txn processes or not.
type TxnInitFunc func(args interface{}) (ret interface{}, errCode int)

type KeyHashFunc func(key string) uint64

type Txn struct {
	ID string

	ctr         *Coordinator
	partsNum    int32
	parts       []*TxnPart
	preparedCnt int64
	done        chan struct{}

	state int32
	mu    sync.Mutex // control txnState

	timeout int64 // Millisecond

	initFunc    TxnInitFunc
	keyHashFunc KeyHashFunc

	// indicate the code when state is aborted
	errorCode int
}

// Abort transaction, i.e. all the particpants of
// the transaction will abort. It must be invoked at most
// once. The txn state has been set to StateAborted before
// this function.
//
// It's mutually exclusive with commitTxn.
func (txn *Txn) abortTxn() {
	// TODO
	var wc sync.WaitGroup
	for i := 0; i < len(txn.parts); i++ {
		wc.Add(1)
		txnPart := txn.parts[i]

		// Abort all the parts of the txn.
		go func(txnPart *TxnPart) {
			// Set states of all parts of txn to StateAborted.
			atomic.StoreInt32(&txnPart.state, StateTxnPartAborted)
			args := AbortArgs{TxnPartID: txnPart.ID}
			var reply AbortReply
			var ok = false
			for !ok {
				ok = call(txnPart.Remote, "Participant.Abort", args, &reply)
			}
			wc.Done()
		}(txnPart)
	}
	wc.Wait()
	atomic.StoreInt32(&txn.state, StateTxnAborted)
}

// Invoked in the following conditions:
// 1. One part of Txn informed StateAborted.
// 2. Not all parts of Txn get prepared in a timeout period.
// 3. Txn actively aborts.
//
// txnPartIdx < 0 in 2 and 3 conditions.
func (txn *Txn) abortTxnPart(partIdx int, errCode int) {
	// TODO
	if partIdx >= 0 {
		txnPart := txn.parts[partIdx]
		txnPart.errCode = errCode
		atomic.StoreInt32(&txnPart.state, StateTxnPartAborted)
	}

	// Make sure abortTxn will be triggered only once.
	txn.mu.Lock()
	defer txn.mu.Unlock()
	state := atomic.LoadInt32(&txn.state)
	if state != StateTxnAborted {
		// assert txnState != StateCommitted

		txn.done <- struct{}{}
		go txn.abortTxn()
	}
}

// Invoked when one part of Txn informed StatePrepared.
func (txn *Txn) prepareTxnPart(partIdx int, errCode int) {

	txnPart := txn.parts[partIdx]
	txnPart.errCode = errCode
	swapped := atomic.CompareAndSwapInt32(&txnPart.state,
		StateTxnPartWorking, StateTxnPartPrepared)

	// A new TxnPart get prepared.
	if swapped {
		// Make sure CommitTxn will be triggered only once.
		cnt := atomic.AddInt64(&txn.preparedCnt, 1)

		if cnt == int64(len(txn.parts)) {
			txn.mu.Lock()
			defer txn.mu.Unlock()
			if atomic.LoadInt32(&txn.state) != StateTxnAborted {
				txn.done <- struct{}{}
				go txn.commitTxn()
			}
		}
	}
}

// Commit the transcation, i.e. all the particpants
// of the transaction will commit. It must be invoked at most
// once. The txn state has been set to StateCommited before
// this function.
//
// It's mutually exclusive with abortTxn.
func (txn *Txn) commitTxn() {
	// TODO
	var wc sync.WaitGroup
	for i := 0; i < len(txn.parts); i++ {
		wc.Add(1)
		txnPart := txn.parts[i]
		// Commit all the parts of the txn.
		go func(txnPart *TxnPart) {
			// Set states of all parts of txn to StateCommitted.
			atomic.StoreInt32(&txnPart.state, StateTxnPartCommitted)
			args := CommitArgs{TxnPartID: txnPart.ID}
			var reply CommitReply
			var ok = false
			for !ok {
				ok = call(txnPart.Remote, "Participant.Commit", args, reply)
			}
			wc.Done()
		}(txnPart)
	}
	atomic.StoreInt32(&txn.state, StateTxnCommitted)
}

// Wait for all participants to enter the prepared states.
// If it receives all the prepared states before the
// timeout, commitTxn() may be invoked. Otherwise abortTxn()
// may be be invoked.
//
// Notice: We use "may", because the conditions of
// CommitTxn or AbortTxn coule be satisfied when the
// paticpants call Prepared or Aborted asynchronously.
func (txn *Txn) waitAllPartsPrepared() {
	// TODO
	select {
	case <-time.Tick(time.Millisecond * time.Duration(txn.timeout)):
		{
			fmt.Println("Abort because of timeout")
			txn.errorCode = ErrTimeout
			txn.abortTxnPart(-1, 0)
		}
	case <-txn.done:
		{
			return
		}
	}
}

func nrand() string {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return strconv.FormatInt(x, 10)
}

func (txn *Txn) SetTxnPart(key, callName string, callArgs interface{}) {
	shard := int(txn.keyHashFunc(key)) % len(txn.ctr.ppts)
	txn.parts = append(txn.parts, nil)
	id := nrand()
	part := &TxnPart{ID: id, TxnID: txn.ID, Idx: int(txn.partsNum),
		Key: key, Shard: shard, Remote: txn.ctr.ppts[shard],
		CallName: callName, CallArgs: callArgs, InitRet: nil,
		txn: txn, state: StateTxnCreated, undoDone: false, canAbort: false,
	}
	txn.parts[part.Idx] = part
	atomic.AddInt32(&txn.partsNum, 1)
}

type TxnPart struct {
	ID    string // ID of TxnPart
	TxnID string // ID of the corresponding Txn
	Idx   int    // Index of TxnPart among the parts of Txn

	Key   string
	Shard int // Index of shards
	// Remote address(host:port) of the corresponding participant
	// of the its shard.
	Remote string

	// --------------
	// Transaction parameters.
	CallName string
	CallArgs interface{}
	InitRet  interface{} // returned by initFunction of Txn
	// --------------

	errCode int

	// rollbacker is decided after caller is executed.
	rollbacker Rollbacker

	txn      *Txn
	state    int32
	canAbort bool
	undoDone bool
}

// Undo the executed opertions. It must be invoked at most
// once.
//
// Undo operations should be idempotent, since Undo() may be
// invoked again if a failure occurs during it.
// func (txn *TxnPart) Undo() {
// 	// TODO
// 	// Idempotent undo procedure
// 	fmt.Println("Idempotent undo procedure")
// 	txn.undoDone = true
// }
