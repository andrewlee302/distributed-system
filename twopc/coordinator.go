package twopc

//
// Coordinator is the role of two-phase commit protocol.
//
// We assume that at least one participant is OK. So
// timeout could be started to calculated from receiving
// the 1st StatePrepared.
//
// We assume that there is no fault in the Coordinator,
// including the communication errors (timeout, socket
// close, etc), the process and power failures.
//
// CommitTxn and AbortTxn are mutually exclusive, which is
// the essence of two-phase commit protocol. So please notice
// AbortTxn because of timeout and simultaneous CommitTxn.
// Also, the two functions must be invoked at most once.
//
// Transcation state (txnState). Initial state is StateWorking.
// StateWorking could be transferred to StatePrepared
// if 1st received state is StatePrepared.
// StateWorking could be transferred to StateAborted
// if 1st received state is StateAborted.
// StatePrepared could be transferred to StateAborted
// if receving any StateAborted.
// StatePrepared could be transferred to StateCommitted
// if receving all StateCommitted.
//
// API
// StateTxn
// Abort

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

// Coordinator is the manager role of two-phase commit.
type Coordinator struct {
	mu   sync.Mutex
	l    net.Listener
	rpcs *rpc.Server
	pa   *util.ResourcePoolsArray

	coord string   // coordinator address
	ppts  []string // participants addresses

	network string
	txnsMu  sync.RWMutex
	txns    map[string]*Txn
}

const CoordClientMaxSizeForOnePpt = 100

// NewCoordinator init a Coordinator service.
func NewCoordinator(network, coord string, ppts []string) *Coordinator {
	ctr := &Coordinator{network: network, coord: coord, ppts: ppts,
		txns: make(map[string]*Txn)}
	news := make([]func() util.Resource, len(ppts))
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
		log.Fatal("listen error: ", e)
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

// RegisterService registers the service onto the rpc calls.
func (ctr *Coordinator) RegisterService(service interface{}) {
	ctr.rpcs.Register(service)
}

// NewTxn initialize a new Txn. It's thread-safe.
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

type TxnState struct {
	State   int32
	ErrCode int
}

// StateTxn return the latest state of the transcation.
func (ctr *Coordinator) StateTxn(txnID *string, reply *TxnState) error {
	txn := ctr.txnByID(*txnID)
	reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	return nil
}

// StateTxn return the latest state of the transcation.
func (ctr *Coordinator) SyncTxnEnd(txnID *string, reply *TxnState) error {
	txn := ctr.txnByID(*txnID)
	reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	retryCnt := 0
	for reply.State != StateTxnAborted && reply.State != StateTxnCommitted {
		retryCnt++
		waitTime := int64(math.Pow(2, float64(retryCnt)))
		waitTime = 0
		time.Sleep(time.Millisecond * time.Duration(waitTime))
		reply.State, reply.ErrCode = atomic.LoadInt32(&txn.state), txn.errCode
	}
	return nil
}

// StartTxn starts the transcation on all the particpants.
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

// // Abort triggers when users want to actively abort the
// // transaction in some conditions.
// func (ctr *Coordinator) Abort(txnID string) {
// 	txn := ctr.txnByID(txnID)
// 	txn.errorCode = ErrTxnUserAbort
// 	txn.abortTxnPart(-1, 0)
// }

// ------------ RPC calls START -------------

// InformPrepared is invoked when some participant informs
// the prepared state.
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

// InformAborted is invoked when some participant informs
// the aborted state.
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

// Kill tell the coordinator to shut itself down.
// for testing.
// please do not change these two functions.
func (ctr *Coordinator) Kill() {
	if ctr.l != nil {
		ctr.l.Close()
	}
}
