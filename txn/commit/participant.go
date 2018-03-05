package commit

//
// Participant is the role of two-phase commit protocol.
//
// Transcation state (state). Initial state is StateWorking.
// StateWorking could be transferred to StatePrepared when
// Prepared() is invoked and
// StateAborted.
// StateWorking could be transferred to StateAborted
// if 1st received state is StateAborted.
// StatePrepared could be transferred to StateAborted
// if receving any StateAborted.
// StatePrepared could be transferred to StateCommitted
// if receving all StateCommitted.

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"sync"
	"sync/atomic"
	"syscall"
)

// Participant is the executed role of two-phase commit.
type Participant struct {
	mu         sync.Mutex
	l          net.Listener
	dead       int32 // for testing
	unreliable int32 // for testing
	rpcCount   int32 // for testing

	coord     string // coordinator address
	me        int
	peers     []string
	txnsMu    sync.Mutex
	txnsParts map[string]*TxnPart
	// sbPrepared bool // some particpant has prepared

	callerMap map[string]Caller
}

// RegisterCaller register a caller with a unique name,
// which can be used to index the caller.
func (ppt *Participant) RegisterCaller(caller Caller, name string) {
	ppt.callerMap[name] = caller
}

func (ppt *Participant) executeTxnPart(tp *TxnPart) {
	// callerName string, args interface{}, reply *int
	caller, ok := ppt.callerMap[tp.CallName]
	if !ok {
		panic("Invalid call: " + tp.CallName)
	}
	tp.callFlag, tp.rollbacker = caller.Call(tp.CallArgs, tp.InitRet)
}

// SubmitTxnPart submit the TxnPart to the participant and start it.
// @reply could be nil.
func (ppt *Participant) SubmitTxnPart(tp *TxnPart, reply *struct{}) error {
	tp.state = StateTxnPartWorking
	ppt.txnsMu.Lock()
	ppt.txnsParts[tp.ID] = tp
	ppt.txnsMu.Unlock()
	go func() {
		ppt.executeTxnPart(tp)
		if tp.callFlag != 0 {
			// Call failed.
			ppt.aborted(tp)
		} else {
			// Call successfully.
			ppt.prepared(tp)
		}
	}()
	return nil
}

// Prepared is the action when the participant declares
// the prepared state for the part of the transaction.
//
// It will be actively invoked when the business logic
// think the part of the transcation is ok.
func (ppt *Participant) prepared(tp *TxnPart) {
	atomic.StoreInt32(&tp.state, StateTxnPartPrepared)
	// assert ppt.me == tp.Shard
	args := PreparedArgs{TxnPartIdx: tp.Idx, TxnID: tp.TxnID, Flag: tp.callFlag}
	var reply PreparedReply
	var ok = false
	for !ok {
		ok = call(ppt.coord, "Coordinator.InformPrepared", args, &reply)
	}
}

// Aborted is the action when the participant aborts
// because of some conditions of business logics.
//
// It should be actively invoked when the business logic
// has to abort the transcation in some conditions. For
// example, the withdraw account doesn't have enough money
// considering transferring money between two accounts.
func (ppt *Participant) aborted(tp *TxnPart) {
	ppt.abort(tp)
	args := AbortedArgs{TxnPartIdx: tp.Idx, TxnID: tp.TxnID, Flag: tp.callFlag}
	var reply AbortedReply
	var ok = false
	for !ok {
		ok = call(ppt.coord, "Coordinator.InformAborted", args, &reply)
	}
}

// Abort is invoked by coordinator.
func (ppt *Participant) Abort(args *AbortArgs, reply *AbortReply) error {
	tp := ppt.endTxnPart(args.TxnPartID)
	// Abort method could be called not only once.
	if tp != nil {
		ppt.abort(tp)
	}
	return nil
}

// Commit is invoked by coordinator.
func (ppt *Participant) Commit(args *CommitArgs, reply *CommitReply) error {
	tp := ppt.endTxnPart(args.TxnPartID)
	// Commit method could be called not only once.
	if tp != nil {
		atomic.StoreInt32(&tp.state, StateTxnPartCommitted)
	}
	return nil
}

func (ppt *Participant) endTxnPart(txnPartID string) *TxnPart {
	ppt.txnsMu.Lock()
	tp := ppt.txnsParts[txnPartID]
	delete(ppt.txnsParts, txnPartID)
	ppt.txnsMu.Unlock()
	return tp
}

func (ppt *Participant) abort(tp *TxnPart) {
	atomic.StoreInt32(&tp.state, StateTxnPartAborted)
	if tp.canAbort == false {
		tp.canAbort = true
		tp.rollbacker.Rollback()
	}
}

// NewParticipant init a participant service.
func NewParticipant(peers []string, me int, coord string) *Participant {
	ppt := &Participant{peers: peers, me: me, coord: coord,
		txnsParts: make(map[string]*TxnPart), callerMap: make(map[string]Caller)}
	l, e := net.Listen("unix", peers[me])
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	ppt.l = l
	rpcs := rpc.NewServer()
	rpcs.Register(ppt)

	// Don't change any of the following code,
	// or do anything to subvert it.

	// Create a thread to accept RPC connections
	go func() {
		for ppt.isdead() == false {
			conn, err := ppt.l.Accept()
			if err == nil && ppt.isdead() == false {
				if ppt.isunreliable() && (rand.Int63()%1000) < 100 {
					// discard the request.
					conn.Close()
				} else if ppt.isunreliable() && (rand.Int63()%1000) < 200 {
					// process the request but force discard of reply.
					c1 := conn.(*net.UnixConn)
					f, _ := c1.File()
					err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
					if err != nil {
						fmt.Printf("shutdown: %v\n", err)
					}
					atomic.AddInt32(&ppt.rpcCount, 1)
					go rpcs.ServeConn(conn)
				} else {
					atomic.AddInt32(&ppt.rpcCount, 1)
					go rpcs.ServeConn(conn)
				}
			} else if err == nil {
				conn.Close()
			}
			if err != nil && ppt.isdead() == false {
				fmt.Printf("Participant(%v) accept: %v\n", me, err.Error())
			}
		}
	}()
	return ppt
}

// Kill tell the peer to shut itself down.
// for testing.
// please do not change these two functions.
func (ppt *Participant) Kill() {
	atomic.StoreInt32(&ppt.dead, 1)
	if ppt.l != nil {
		ppt.l.Close()
	}
}

// Has this peer been asked to shut down?
func (ppt *Participant) isdead() bool {
	return atomic.LoadInt32(&ppt.dead) != 0
}

// Please do not change these two functions.
func (ppt *Participant) setunreliable(what bool) {
	if what {
		atomic.StoreInt32(&ppt.unreliable, 1)
	} else {
		atomic.StoreInt32(&ppt.unreliable, 0)
	}
}

func (ppt *Participant) isunreliable() bool {
	return atomic.LoadInt32(&ppt.unreliable) != 0
}
