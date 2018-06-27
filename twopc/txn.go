package twopc

import (
	"crypto/rand"
	"distributed-system/util"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// BUG():.

// States of Txn.
const (
	// Created state in NewTxn.
	StateTxnCreated = iota
	// Initial function is executed successfully.
	StateTxnInit
	// The first msg from Participant for a Txn is Prepared msg.
	StateTxnPreparing
	// All TxnParts have been informed of the Prepared msgs.
	StateTxnCommitted
	// Received one or more Aborted msgs, or timeout after receiving the
	// first Prepared msg.
	StateTxnAborted
)

// States of TxnPart.
const (
	// Participant received the TxnPart before executing it.
	StateTxnPartWorking = iota
	// TxnPart is executed successfully.
	StateTxnPartPrepared
	// TxnPart is executed with an error.
	StateTxnPartAborted
	// TxnPart is informed that all TxnPart have been gotten Prepared states.
	StateTxnPartCommitted
)

// Special error codes for Txn. They are less than 0.
// 0 means success.
// User-defined error code must be more than 0.
const (
	ErrTxnTimeout   = -1
	ErrTxnUserAbort = -2
)

// Caller represents one part of transaction logic executed by the specific
// Participant. It's implemented by the library user and registered in the
// Participant by RegisterCaller function.
//
// ErrCode decide whether the Participant sends back the StatePrepared or
// StateAborted msg. It must be more than 0. Rb is the rollbacker, which is
// execuetd while errCode is not 0, i.e. the txn should be aborted and the
// txnPart should rollback the state.
//
// InitRet is the return value of initial function of the txn. It is filled
// in the Coordinator before the txnPart is submitted to the Participant. It
// could be nil.
type Caller interface {
	Call(initRet interface{}) (errCode int, rb Rollbacker)
}

// CallFunc is a Caller. User could use the function directly as a Caller.
type CallFunc func(initRet interface{}) (errCode int, rb Rollbacker)

// Call makes CallFunc implement the Caller interface.
func (f CallFunc) Call(initRet interface{}) (errCode int, rb Rollbacker) {
	errCode, rb = f(initRet)
	return
}

// Rollbacker represents rollback logic of the txnPart on the specific
// Participant. It related to the specific Caller. If the errCode of Caller
// is not 0, then the rollbacker will be executed to redo the state changes.
// It's implemented by the library user and returned in the Caller's
// implementation.
type Rollbacker interface {
	Rollback()
}

// RollbackFunc is a Rollbacker. User could use the function directly as a
// Rollbacker.
type RollbackFunc func()

// Rollback makes RollbackFunc implement the Rollbacker interface.
func (f RollbackFunc) Rollback() {
	f()
}

// BlankRollbackFunc is the Rollbacker with blank logic.
var BlankRollbackFunc RollbackFunc = func() {}

// TxnInitFunc is the initialization before Txn processing. The returning
// errCode indicates the state of the procedure, which decides whether the
// following Txn processes or not. If it's 0, then do the next. Otherwise,
// stop the txn.
type TxnInitFunc func(args interface{}) (ret interface{}, errCode int)

// BlankTxnInitFunc is the blank TxnInitFunc without any logics and return 0.
var BlankTxnInitFunc TxnInitFunc = func(args interface{}) (ret interface{}, errCode int) {
	ret = nil
	errCode = 0
	return
}

// KeyHashFunc is the hash func for distributing the TxnParts.
type KeyHashFunc func(key string) uint64

// DefaultKeyHashFunc is the default KeyHashFunc.
var DefaultKeyHashFunc KeyHashFunc = func(key string) uint64 {
	var hash uint64 = 0
	for i := 0; i < len(key); i++ {
		hash = 31*hash + uint64(key[i])
	}
	return hash & (1<<63 - 1)
}

// Txn is the structure for a transaction, which is created by Coordinator
// and be controlled by binding functions.
type Txn struct {
	ID string

	ctr         *Coordinator
	partsNum    int32
	parts       []*TxnPart
	preparedCnt int64
	done        chan struct{}

	state int32
	mu    sync.Mutex // control txnState

	timeoutMs int64 // Millisecond

	initFunc    TxnInitFunc
	keyHashFunc KeyHashFunc

	// indicate the code when state is aborted
	errCode int
}

// Abort transaction, i.e. all the particpants of the transaction will abort.
// It must be invoked at most once. The txn state has been set to
// StateAborted before this function.
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
				ok = util.RPCPoolArrayCall(txn.ctr.pa, txnPart.Shard, "Participant.Abort", args, &reply)
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
		// !!! OR operator
		txn.errCode = txn.errCode | txnPart.errCode
		// fmt.Println("abortTxnPart", txn.errCode)
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

// commitTxn commits the transcation, i.e. all the particpants of the
// transaction will commit. It must be invoked at most once. The txn state
// has been set to StateCommited before this function.
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
				ok = util.RPCPoolArrayCall(txn.ctr.pa, txnPart.Shard, "Participant.Commit", args, &reply)
			}
			wc.Done()
		}(txnPart)
	}
	wc.Wait()
	atomic.StoreInt32(&txn.state, StateTxnCommitted)
}

// waitAllPartsPrepared waits for all participants to enter the prepared
// states. If it receives all the prepared states before the timeout,
// commitTxn() may be invoked. Otherwise abortTxn() may be be invoked.
//
// Notice: We use "may", because the conditions of CommitTxn or AbortTxn
// coule be satisfied when the paticpants call Prepared or Aborted
// asynchronously.
func (txn *Txn) waitAllPartsPrepared() {
	// TODO
	select {
	case <-time.Tick(time.Millisecond * time.Duration(txn.timeoutMs)):
		{
			fmt.Println("Abort because of timeout", time.Millisecond*time.Duration(txn.timeoutMs))
			txn.abortTxnPart(-1, 0)
			txn.errCode = ErrTxnTimeout
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

func (txn *Txn) addTxnPart(shard int, callName string) {
	txn.parts = append(txn.parts, nil)
	id := nrand()
	part := &TxnPart{ID: id, TxnID: txn.ID, Idx: int(txn.partsNum),
		Shard: shard, Remote: txn.ctr.ppts[shard],
		CallName: callName, InitRet: nil,
		txn: txn, state: StateTxnCreated, undoDone: false, canAbort: false,
	}
	txn.parts[part.Idx] = part
	atomic.AddInt32(&txn.partsNum, 1)
}

// AddTxnPart adds TxnPart into the Txn. Key decides which specific
// Participant will execute the TxnPart. CallName is the function name
// binding to the Particpant.
func (txn *Txn) AddTxnPart(key, callName string) {
	shard := int(txn.keyHashFunc(key)) % len(txn.ctr.ppts)
	txn.addTxnPart(shard, callName)
}

// BroadcastTxnPart adds TxnPart into the Txn. The TxnPart will be executed
// on all Participants instead of the specific Participant. CallName is the
// function name binding to the Particpant.
//
// It is usually be used when we don't know which Partipant should execute
// the TxnPart logic.
func (txn *Txn) BroadcastTxnPart(callName string) {
	for i := 0; i < len(txn.ctr.ppts); i++ {
		txn.addTxnPart(i, callName)
	}
}

// Start to execute the transcation. Firstly initilize the Txn by initArgs.
// If the return code is 0, the TxnParts will be submitted into the
// corresponding shards on specific Participators. Otherwise, abort the
// Txn immediately.
func (txn *Txn) Start(initArgs interface{}) {
	// TODO
	ret, errCode := txn.initFunc(initArgs)

	// stop the txn
	if errCode != 0 {
		txn.errCode = errCode
		atomic.StoreInt32(&txn.state, StateTxnAborted)
		return
	}
	atomic.StoreInt32(&txn.state, StateTxnInit)

	for _, txnPart := range txn.parts {
		go func(txnPart *TxnPart) {
			if ret != nil {
				txnPart.InitRet = ret
			}
			// fmt.Println("here", *txnPart)
			util.RPCPoolArrayCall(txn.ctr.pa, txnPart.Shard, "Participant.SubmitTxnPart", txnPart, &struct{}{})
		}(txnPart)
	}
}

// TxnPart is one part of the transaction. One transcation is made up for
// several TxnParts, function named by CallName will be executed on the specific
// participant and errCode returned will affect whether the participant sends
// back StatePrepared or StateAborted msg. The rollbacker will be executed if
// the corresponding txn aborted.
type TxnPart struct {
	// ID of TxnPart.
	ID string

	// ID of the corresponding Txn.
	TxnID string

	// Idx is the index of TxnPart among the parts of Txn.
	Idx int

	// Shard is th index of shards
	Shard int

	// Remote address(host:port) of the corresponding participant
	// of the its shard.
	Remote string

	// CallName is the binding name of the function.
	CallName string

	// InitRet is the return value of initFunction of Txn.
	InitRet interface{}

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
