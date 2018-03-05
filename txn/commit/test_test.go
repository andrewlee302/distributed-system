package commit

import (
	"encoding/gob"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
)

func port(tag string, host int) string {
	s := "/var/tmp/824-"
	s += strconv.Itoa(os.Getuid()) + "/"
	os.Mkdir(s, 0777)
	s += "px-"
	s += strconv.Itoa(os.Getpid()) + "-"
	s += tag + "-"
	s += strconv.Itoa(host)
	return s
}
func cleanup(ppts []*Participant, coord *Coordinator) {
	for i := 0; i < len(ppts); i++ {
		if ppts[i] != nil {
			ppts[i].Kill()
		}
	}
	coord.Kill()
}

func test(t *testing.T, ppts []*Participant, coord *Coordinator) {

}

func TestBasic(t *testing.T) {
	runtime.GOMAXPROCS(4)
	const nppt = 2
	const timeout = 1000 // ms
	ppts := make([]*Participant, nppt)
	pptsPorts := make([]string, nppt)
	dbs := make([]BasicDataBase, nppt)
	var coord *Coordinator

	coordPort := port("basic_c", 0)

	for i := 0; i < nppt; i++ {
		pptsPorts[i] = port("basic_p", i)
	}
	for i := 0; i < nppt; i++ {
		dbs[i] = BasicDataBase{dataset: make(map[int]int)}
		ppts[i] = NewParticipant(pptsPorts, i, coordPort)
		ppts[i].RegisterCaller(CallFunc(dbs[i].Add), "Add")
		ppts[i].RegisterCaller(CallFunc(dbs[i].Set), "Set")
	}
	coord = NewCoordinator(pptsPorts, coordPort)

	fmt.Printf("Test: Basic")

	txn := coord.NewTxn(initFunc, keyHashFunc, 10000)

	gob.Register(TxnArgs{})
	txn.SetTxnPart("1", "Set", TxnArgs{Idx: 1, SetValue: 2})
	txn.SetTxnPart("2", "Add", TxnArgs{Idx: 2})
	coord.StartTxn(txn, 0)

	for coord.StateTxn(txn.ID) != StateTxnCommitted {
	}
	// TODO
	cleanup(ppts, coord)
}

type BasicDataBase struct {
	dataset map[int]int
}

type TxnArgs struct {
	Idx, SetValue int
}

func (base *BasicDataBase) Set(args interface{}, initRet interface{}) (reply int, rbf Rollbacker) {
	txnArgs := args.(TxnArgs)
	origin := base.dataset[txnArgs.Idx]
	rbf = RollbackFunc(func() {
		base.dataset[txnArgs.Idx] = origin
	})
	base.dataset[txnArgs.Idx] = txnArgs.SetValue
	reply = 0
	return
}

func (base *BasicDataBase) Add(args interface{}, initRet interface{}) (reply int, rbf Rollbacker) {
	txnArgs := args.(TxnArgs)
	delta := initRet.(int)
	origin := base.dataset[txnArgs.Idx]
	rbf = RollbackFunc(func() {
		base.dataset[txnArgs.Idx] = origin
	})
	base.dataset[txnArgs.Idx] += delta
	reply = 0
	return
}

var initFunc TxnInitFunc = func(args interface{}) (ret interface{}, flag bool) {
	if v, ok := args.(int); !ok {
		ret, flag = nil, false
	} else {
		ret, flag = v+1, true
	}
	return
}

var keyHashFunc KeyHashFunc = func(key string) uint64 {
	return uint64(key[0])
}
