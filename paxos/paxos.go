/*
Package paxos provides paxos components for the strong consistency.

The paxos peers as a whole manage a sequential agreed values. Every paxos peer
should be wrapped in an application instance. All the instances build up a
whole system to keep the data consistency. Notice that the set of peers is
fixed.

The data decided by the paxos peers could be operations, which could
constitute a State Machine to reveal the state transferring of the actual
data. The typical application is the key-value database.


Fault-tolerance Model

The crash and restart won't happen, because the memory is not consistent.
But the followings should be considered.
 * The network partition.
 * Message loss or long latency in a channel.

API

The paxos library provides 6 interfaces.

Initialize paxos peer.
 px = paxos.Make(peers []string, me string) -- Create a paxos peer in the
   specific group.

Consensus interfaces (Core API).
 px.Start(seq int, v interface{}) -- Start agreement on new instance with the
   sequence number. v is the proposal value which maybe not be chosen.
   Because there is another value has been chosen in the progress.
 px.Status(seq int) (Fate, v interface{}) -- Get the info of the instance
   with the sequence number. Fate is the status: Chosen, Pending or
   Forgotten. v is the chosen value.

The following three are for the memory efficience.
 px.Done(seq int) -- Tell all the peers that all instances <= seq could be
   forgotten to release some memory space.
 px.Max() int -- Get the highest instance seq known, or -1.
 px.Min() int -- Get the lowest instance seq that all the lower seq instances
   than it have been forgotten.
*/
package paxos

import (
	"distributed-system/util"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Fate is the status of the paxos instance which could be viewed by
// px.Status() interface.
type Fate int

// Fate of the paxos instance.
const (
	// The value on the instance is chosen.
	Chosen Fate = iota + 1
	// The value has been chosen yet.
	Pending
	// The value one the instance is chosen but forgotten.
	Forgotten
)

// Paxos is the paxos peer of one paxos group. Every paxos peer is an isolated
// process (of course a thread or a goroutine). It has more than one paxos
// instances with the sequential numbers. Every instance burden the three
// roles (agents in "Paxos made simple") : Proposer, Acceptor and Leaner.
type Paxos struct {
	mu         sync.Mutex
	l          net.Listener
	dead       int32 // for testing
	unreliable int32 // for testing
	rpcCount   int32 // for testing
	peers      []string
	me         int // index into peers[]

	// Your data here.
	proposerMgr *proposerManager
	acceptorMgr *acceptorManager

	chosenValues map[int]interface{}

	doneSeqs     []int // non-local, except doneSeqs[me]
	minDoneSeq   int   // the minimal in doneSeqs
	minDoneIndex int   // the index of the minimal in doneSeqs
}

type proposerManager struct {
	px           *Paxos
	mu           sync.Mutex
	peers        []string
	me           int // index of the proposers
	proposers    map[int]*Proposer
	seqMax       int
	seqChosenMax int
}

type acceptorManager struct {
	cp        *util.ResourcePoolsArray
	mu        sync.Mutex
	acceptors map[int]*Acceptor
	seqMax    int
}

// Acceptor is the Acceptor role for one paxos instance.
type Acceptor struct {
	mu sync.Mutex
	// init: -1, -1, ""
	nP int
	nA int
	vA interface{}
}

// Proposer is the Proposer role for one paxos instance.
type Proposer struct {
	mgr          *proposerManager
	seq          int
	proposeValue interface{}
	isDead       bool
	proposalN    int
}

// PrepareArgs is Prepare RPC args.
type PrepareArgs struct {
	Seq int
	N   int
}

// PrepareReply is Prepare RPC reply.
type PrepareReply struct {
	N    int // for choosing next proposing number
	NA   int
	VA   interface{}
	Succ bool
}

// AcceptArgs is Accept RPC args.
type AcceptArgs struct {
	Seq int
	N   int
	V   interface{}
}

// AcceptReply is Accept RPC reply.
type AcceptReply struct {
	N    int // for choosing next proposing number
	Succ bool
}

// ChooseArgs is Choose RPC args.
type ChooseArgs struct {
	Seq int
	V   interface{}
}

// ChooseReply is Choose RPC reply.
type ChooseReply bool

// SeqArgs is UpdateDoneSeqs args.
type SeqArgs struct {
	Sender int
	Seq    int
}

// SeqReply is UpdateDoneSeqs reply.
type SeqReply bool

func (proposerMgr *proposerManager) runProposer(seq int, v interface{}) {
	proposerMgr.mu.Lock()
	defer proposerMgr.mu.Unlock()
	if _, ok := proposerMgr.proposers[seq]; !ok {
		if seq > proposerMgr.seqMax {
			proposerMgr.seqMax = seq
		}
		prop := &Proposer{mgr: proposerMgr, seq: seq, proposeValue: v, isDead: false, proposalN: proposerMgr.me}
		proposerMgr.proposers[seq] = prop
		go func() {
			prop.Propose()
		}()
	}
}

func (acceptorMgr *acceptorManager) getInstance(seq int) *Acceptor {
	acceptorMgr.mu.Lock()
	defer acceptorMgr.mu.Unlock()
	acceptor, ok := acceptorMgr.acceptors[seq]
	if !ok {
		if seq > acceptorMgr.seqMax {
			acceptorMgr.seqMax = seq
		}
		acceptor = &Acceptor{nP: -1, nA: -1, vA: nil}
		acceptorMgr.acceptors[seq] = acceptor
	}
	return acceptor
}

// Prepare is RPC routine invoked by Paxos peers.
func (px *Paxos) Prepare(args *PrepareArgs, reply *PrepareReply) error {
	acceptor := px.acceptorMgr.getInstance(args.Seq)
	acceptor.mu.Lock()
	defer acceptor.mu.Unlock()
	if args.N > acceptor.nP { // optimization in "Paxos made simple".
		reply.Succ = true
		acceptor.nP = args.N // control the future instead of predicating the future.
		reply.N = args.N
		reply.NA = acceptor.nA
		reply.VA = acceptor.vA
	} else {
		reply.Succ = false
		reply.N = acceptor.nP
	}
	return nil
}

// Accept is RPC routine invoked by Paxos peers.
func (px *Paxos) Accept(args *AcceptArgs, reply *AcceptReply) error {
	acceptor := px.acceptorMgr.getInstance(args.Seq)
	acceptor.mu.Lock()
	defer acceptor.mu.Unlock()
	if args.N >= acceptor.nP {
		reply.Succ = true
		acceptor.nP = args.N
		acceptor.nA = args.N
		acceptor.vA = args.V
		reply.N = args.N
	} else {
		reply.Succ = false
		reply.N = acceptor.nP
	}
	return nil
}

// Choose is RPC routine invoked by Paxos peers.
func (px *Paxos) Choose(args *ChooseArgs, reply *ChooseReply) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	// assertion: it must be an idempotent operation
	px.chosenValues[args.Seq] = args.V
	*reply = ChooseReply(true)
	return nil
}

// Propose proposes the given value on the specific instance until the
// instance makes a chosen value or gets dead.
func (proposer *Proposer) Propose() {
	peersNum := len(proposer.mgr.peers)
	majorityNum := peersNum/2 + 1

	for !proposer.isDead {
		nextProposeNum := proposer.proposalN
		// prepare request
		prepareReplies := make(chan PrepareReply, peersNum)
		prepareBarrier := make(chan bool)
		for me, peer := range proposer.mgr.peers {
			go func(me int, peer string) {
				args := &PrepareArgs{Seq: proposer.seq, N: proposer.proposalN}
				var reply PrepareReply

				// avoid the situation that local rpc is fragile, however the acceptor should prepare value that itself issued.
				succ := true
				if me != proposer.mgr.me {
					succ = util.Call("unix", peer, "Paxos.Prepare", args, &reply)
				} else {
					proposer.mgr.px.Prepare(args, &reply)
				}
				prepareBarrier <- true
				if succ {
					if reply.Succ {
						prepareReplies <- reply
					} else if reply.N > nextProposeNum {
						// TODO atomic
						nextProposeNum = reply.N
					}
				}
			}(me, peer)
		}

		// barrier
		for i := 0; i < peersNum; i++ {
			<-prepareBarrier
		}

		if len(prepareReplies) >= majorityNum {
			var acceptedValue interface{}
			acceptedProposeNum := -1
			repliesNum := len(prepareReplies)
			for i := 0; i < repliesNum; i++ {
				r := <-prepareReplies
				if r.NA > acceptedProposeNum {
					acceptedProposeNum = r.NA
					acceptedValue = r.VA
				}
			}

			// TODO
			// if nextProposeNum is larger than proposer.proposalN, stop this iteration and set a new proposer.proposalN, then keep going on.

			// accept request
			acceptReplies := make(chan AcceptReply, peersNum)
			acceptBarrier := make(chan bool)
			var acceptReqValue interface{}

			if acceptedValue != nil {
				// accepting has happened, use the accepted value
				acceptReqValue = acceptedValue
			} else {
				// no accept, use own propose value
				acceptReqValue = proposer.proposeValue
			}
			for me, peer := range proposer.mgr.peers {
				go func(me int, peer string) {
					args := &AcceptArgs{Seq: proposer.seq, N: proposer.proposalN, V: acceptReqValue}
					var reply AcceptReply
					// same reason
					succ := true
					if me != proposer.mgr.me {
						succ = util.Call("unix", peer, "Paxos.Accept", args, &reply)
					} else {
						proposer.mgr.px.Accept(args, &reply)
					}
					acceptBarrier <- true
					if succ {
						if reply.Succ {
							acceptReplies <- reply
						} else if reply.N > nextProposeNum {
							nextProposeNum = reply.N
						}
					}
				}(me, peer)
			}

			// barrier
			for i := 0; i < peersNum; i++ {
				<-acceptBarrier
			}

			// Choose procedure is broadcast the learn value
			if len(acceptReplies) >= majorityNum {
				// be sure to get a chosen value
				for me, peer := range proposer.mgr.peers {
					go func(me int, peer string) {
						args := &ChooseArgs{Seq: proposer.seq, V: acceptReqValue}
						var reply ChooseReply
						// same reason
						if me != proposer.mgr.me {
							util.Call("unix", peer, "Paxos.Choose", args, &reply)
							/* For unreliable rpc and controlling resource, it's better
							use the following. It's not necessary, others can lanunch
							the `start` procedure.
							succ := false
							for !succ && !proposer.isDead {
								succ = call(peer, "Paxos.Choose", args, &reply)
								time.Sleep(time.Second)
							}
							*/
						} else {
							proposer.mgr.px.Choose(args, &reply)
						}
					}(me, peer)
				}
				break
			} // end if accept
		} // end if prepare

		tryNum := nextProposeNum/peersNum*peersNum + proposer.mgr.me
		if tryNum > nextProposeNum {
			nextProposeNum = tryNum
		} else {
			nextProposeNum = tryNum + peersNum
		}
		// assertion: next_propose_num become bigger
		if nextProposeNum <= proposer.proposalN || nextProposeNum%peersNum != proposer.mgr.me {
			log.Fatalln("unexpected error!!!")
		}
		proposer.proposalN = nextProposeNum

		// TODO
		// sleep for avoiding thrashing of proposing
		time.Sleep(50 * time.Millisecond)
	}

}

// Start proposes the value v on the instance with seq. It just do a proposal,
// which doesn't mean the proposal will be chosen. It returns immediately.
// The application can check the status of the paxos instance by Status().
func (px *Paxos) Start(seq int, v interface{}) {
	// Your code here.
	// TODO optimize, if we know the history log, just avoid the
	// paxos proposing process.

	// ignore the instance whose seq isn't more than minDoneSeq
	if seq <= px.minDoneSeq {
		return
	}

	px.proposerMgr.runProposer(seq, v)
}

// Done reminds that all the instances (<= seq) have been done and can be
// forgetten.  It aims to free up memory in long-term running paxos-based
// services.
//
// If the application thinks the old records can be discarded,
// the Done() will try to forget the all information about all instances
// (<=seq). We say "try", because only if all the peers ack that one instance
// has been done, then the instance could be forgotten.
//
// Paxos peers need to exchange their highest Done() arguments in order to
// affect the return value of Min(). These exchanges can be piggybacked on
// ordinary Paxos agreement protocol messages, so it is OK if one peers Min
// does not reflect another Peers Done() until after the next instance is
// agreed to.
//
// See more details in Min().
func (px *Paxos) Done(seq int) {
	// Your code here.
	for me, peer := range px.peers {
		go func(me int, peer string) {
			args := &SeqArgs{Seq: seq, Sender: px.me}
			var reply SeqReply
			if me != px.me {
				util.Call("unix", peer, "Paxos.UpdateDoneSeqs", args, &reply)
			} else {
				// the same reason using local invocation instead of RPC
				px.UpdateDoneSeqs(args, &reply)
			}
		}(me, peer)
	}
}

// UpdateDoneSeqs is RPC routine invoked by paxos peers.
func (px *Paxos) UpdateDoneSeqs(args *SeqArgs, reply *SeqReply) error {
	px.mu.Lock()
	defer px.mu.Unlock()
	if args.Seq > px.doneSeqs[args.Sender] {
		px.doneSeqs[args.Sender] = args.Seq
		// minimal changes only when the former one become bigger
		if args.Sender == px.minDoneIndex {
			px.minDoneSeq = args.Seq
			for index, seq := range px.doneSeqs {
				if seq < px.minDoneSeq {
					px.minDoneSeq = seq
					px.minDoneIndex = index
				}
			}

			// release instance

			px.proposerMgr.mu.Lock()
			for s, prop := range px.proposerMgr.proposers {
				if s <= px.minDoneSeq {
					prop.isDead = true
					delete(px.proposerMgr.proposers, s)
				}
			}
			px.proposerMgr.mu.Unlock()

			for s := range px.chosenValues {
				if s <= px.minDoneSeq {
					delete(px.chosenValues, s)
				}
			}

			px.acceptorMgr.mu.Lock()
			for s := range px.acceptorMgr.acceptors {
				if s <= px.minDoneSeq {
					delete(px.acceptorMgr.acceptors, s)
				}
			}
			px.acceptorMgr.mu.Unlock()

		}
	}
	return nil
}

// Max returns the highest instance sequence number known to this paxos peer.
func (px *Paxos) Max() int {
	// Your code here.
	a, b := px.acceptorMgr.seqMax, px.proposerMgr.seqMax
	if a > b {
		return a
	}
	return b
}

// Min returns the lowest instance seq that all the lower seq instances than
// it have been forgotten. In other words, it's 1 + min(Highest(Done_arg)_i).
// Highest(Done_arg)_i is the highest number ever passed to Done() on peer i,
// which is -1 if Done() has never been invoked on peer i.
//
// The fact is that Min() can't increase until all paxos peers have been
// heard from. If a peer is dead or unreachable, other peers Min() will not
// increase even if all reachable peers in its network partition call Done.
// Since when the unreachable peer recovers, it will catch up the missing
// instances, but it won't work it others forget these instances.
func (px *Paxos) Min() int {
	// You code here.
	px.mu.Lock()
	defer px.mu.Unlock()
	return px.minDoneSeq + 1
}

// Status gets the status of the instance with the seq to reveal whether the
// value is chosen and if so what the chosen value is. Status() just inspects
// the local peer state, instead of communicating with other paxos peers.
func (px *Paxos) Status(seq int) (Fate, interface{}) {
	// Your code here.
	px.mu.Lock()
	defer px.mu.Unlock()
	if value, ok := px.chosenValues[seq]; seq <= px.minDoneSeq {
		return Forgotten, nil
	} else if ok {
		return Chosen, value
	} else {
		return Pending, nil
	}
}

//
// -------------------- Start ---------------------
// Please don't change the following four functions.

// Kill shuts down this peer to make network failures for testing.
func (px *Paxos) Kill() {
	atomic.StoreInt32(&px.dead, 1)
	if px.l != nil {
		px.l.Close()
	}
}

func (px *Paxos) isdead() bool {
	return atomic.LoadInt32(&px.dead) != 0
}

func (px *Paxos) setunreliable(what bool) {
	if what {
		atomic.StoreInt32(&px.unreliable, 1)
	} else {
		atomic.StoreInt32(&px.unreliable, 0)
	}
}

func (px *Paxos) isunreliable() bool {
	return atomic.LoadInt32(&px.unreliable) != 0
}

// -------------------- End ---------------------

// Make creates a paxos peer and start the service. Peers are the addresses
// of all the paxos peers (including itself). Me is the index of the peer of
// itself.
func Make(peers []string, me int, rpcs *rpc.Server) *Paxos {
	px := &Paxos{}
	px.peers = peers
	px.me = me

	// Your initialization code here.
	px.chosenValues = make(map[int]interface{})

	px.doneSeqs = make([]int, len(px.peers))
	for i := 0; i < len(px.peers); i++ {
		px.doneSeqs[i] = -1
	}
	px.minDoneSeq = -1
	px.minDoneIndex = 0

	news := make([]func() util.Resource, len(peers))
	for i := 0; i < len(peers); i++ {
		addr := peers[i]
		news[i] = func() util.Resource {
			return util.DialServer("unix", addr)
		}
	}
	px.proposerMgr = &proposerManager{peers: peers, me: me, proposers: make(map[int]*Proposer), seqMax: -1, seqChosenMax: -1, px: px}
	px.acceptorMgr = &acceptorManager{acceptors: make(map[int]*Acceptor), seqMax: -1}

	if rpcs != nil {
		// caller will create socket &c
		rpcs.Register(px)
	} else {
		rpcs = rpc.NewServer()
		rpcs.Register(px)

		// prepare to receive connections from clients.
		// change "unix" to "tcp" to use over a network.
		os.Remove(peers[me]) // only needed for "unix"
		l, e := net.Listen("unix", peers[me])
		if e != nil {
			log.Fatal("listen error: ", e)
		}
		px.l = l

		// please do not change any of the following code,
		// or do anything to subvert it.

		// create a thread to accept RPC connections
		go func() {
			for px.isdead() == false {
				conn, err := px.l.Accept()
				if err == nil && px.isdead() == false {
					if px.isunreliable() && (rand.Int63()%1000) < 100 {
						// discard the request.
						conn.Close()
					} else if px.isunreliable() && (rand.Int63()%1000) < 200 {
						// process the request but force discard of reply.
						c1 := conn.(*net.UnixConn)
						f, _ := c1.File()
						err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
						if err != nil {
							fmt.Printf("shutdown: %v\n", err)
						}
						atomic.AddInt32(&px.rpcCount, 1)
						go rpcs.ServeConn(conn)
					} else {
						atomic.AddInt32(&px.rpcCount, 1)
						go rpcs.ServeConn(conn)
					}
				} else if err == nil {
					conn.Close()
				}
				if err != nil && px.isdead() == false {
					fmt.Printf("Paxos(%v) accept: %v\n", me, err.Error())
				}
			}
		}()
	}
	return px
}
