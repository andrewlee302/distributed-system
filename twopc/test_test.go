package twopc

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

func registerStruct() {
	gob.Register(TxnInitRet{})
}

func checkDB(t *testing.T, db *BasicDataBase, idx int, wanted int) {
	v := db.dataset[idx]
	if v != wanted {
		t.Fatalf("db.dataset[%d]; value=%v wanted=%v", idx, v, wanted)
	}
}

func checkErrCode(t *testing.T, errCode, wanted int) {
	if errCode != wanted {
		t.Fatalf("errCode; value=%v wanted=%v", errCode, wanted)
	}
}

func TestBasicSuccess(t *testing.T) {
	runtime.GOMAXPROCS(4)
	const nppt = 2
	const timeout = 1000 // ms
	ppts := make([]*Participant, nppt)
	pptsPorts := make([]string, nppt)
	dbs := make([]*BasicDataBase, nppt)
	var coord *Coordinator

	coordPort := port("basic_c", 0)

	for i := 0; i < nppt; i++ {
		pptsPorts[i] = port("basic_p", i)
	}
	for i := 0; i < nppt; i++ {
		dbs[i] = &BasicDataBase{dataset: make(map[int]int)}
		ppts[i] = NewParticipant("unix", pptsPorts[i], coordPort)
		ppts[i].RegisterCaller(CallFunc(dbs[i].Add), "Add")
		ppts[i].RegisterCaller(CallFunc(dbs[i].Set), "Set")
	}
	coord = NewCoordinator("unix", coordPort, pptsPorts)

	fmt.Printf("Test: Basic Suceess")

	txn := coord.NewTxn(initFunc, keyHashFunc, 10000)
	txnInitArgs := TxnInitArgs{SetIdx: 1, SetValue: 2,
		AddIdx: 2, Delta: 3, A: 0, B: 0}

	registerStruct()
	txn.AddTxnPart("1", "Set")
	txn.AddTxnPart("2", "Add")
	txn.Start(txnInitArgs)
	var txnState TxnState
	coord.StateTxn(&txn.ID, &txnState)
	for txnState.State != StateTxnCommitted {
		coord.StateTxn(&txn.ID, &txnState)
	}
	checkDB(t, dbs[keyHashFunc("1")%nppt], 1, 2)
	checkDB(t, dbs[keyHashFunc("2")%nppt], 2, 3)
	checkErrCode(t, txnState.ErrCode, 0)
	fmt.Printf("  ... Passed\n")
	cleanup(ppts, coord)
}

func TestBasicSFailure(t *testing.T) {
	registerStruct()
	runtime.GOMAXPROCS(4)
	const nppt = 2
	const timeout = 1000 // ms
	ppts := make([]*Participant, nppt)
	pptsPorts := make([]string, nppt)
	dbs := make([]*BasicDataBase, nppt)
	var coord *Coordinator

	coordPort := port("basic_c", 0)

	for i := 0; i < nppt; i++ {
		pptsPorts[i] = port("basic_p", i)
	}
	for i := 0; i < nppt; i++ {
		dbs[i] = &BasicDataBase{dataset: make(map[int]int)}
		ppts[i] = NewParticipant("unix", pptsPorts[i], coordPort)
		ppts[i].RegisterCaller(CallFunc(dbs[i].Add), "Add")
		ppts[i].RegisterCaller(CallFunc(dbs[i].Set), "Set")
	}
	coord = NewCoordinator("unix", coordPort, pptsPorts)

	fmt.Printf("Test: Basic Failure")

	txn := coord.NewTxn(initFunc, keyHashFunc, 10000)
	txnInitArgs := TxnInitArgs{SetIdx: 1, SetValue: 2,
		AddIdx: 2, Delta: 3, A: 1, B: 1}

	txn.AddTxnPart("1", "Set")
	txn.AddTxnPart("2", "Add")
	txn.Start(txnInitArgs)

	var txnState TxnState
	coord.StateTxn(&txn.ID, &txnState)
	for txnState.State != StateTxnAborted {
		coord.StateTxn(&txn.ID, &txnState)
	}
	checkDB(t, dbs[keyHashFunc("1")%nppt], 1, 0)
	checkDB(t, dbs[keyHashFunc("2")%nppt], 2, 0)
	checkErrCode(t, txnState.ErrCode, 1)

	fmt.Printf("  ... Passed\n")
	cleanup(ppts, coord)
}

type BasicDataBase struct {
	dataset map[int]int
}

func (base *BasicDataBase) Set(initRet interface{}) (errCode int, rbf Rollbacker) {
	txnRet := initRet.(TxnInitRet)
	origin := base.dataset[txnRet.SetIdx]
	rbf = RollbackFunc(func() {
		base.dataset[txnRet.SetIdx] = origin
	})
	base.dataset[txnRet.SetIdx] = txnRet.SetValue
	errCode = 0
	return
}

// If base.dataset[txnArgs.Idx] is less than threshold, then failed, when errCode = 1.
func (base *BasicDataBase) Add(initRet interface{}) (errCode int, rbf Rollbacker) {
	txnRet := initRet.(TxnInitRet)
	origin := base.dataset[txnRet.AddIdx]
	rbf = RollbackFunc(func() {
		base.dataset[txnRet.AddIdx] = origin
	})
	if base.dataset[txnRet.AddIdx] < txnRet.Threshold {
		errCode = 1
		fmt.Println("errCode", errCode)
	} else {
		errCode = 0
		base.dataset[txnRet.AddIdx] += txnRet.Delta
	}
	return
}

type TxnInitArgs struct {
	SetIdx   int
	SetValue int
	AddIdx   int
	Delta    int
	A        int
	B        int
}

type TxnInitRet struct {
	TxnInitArgs
	Threshold int
}

// Multiple the param by 2. If it's not a number ,then failed, when errCode = 1.
var initFunc TxnInitFunc = func(args interface{}) (ret interface{}, errCode int) {
	txnInitArgs := args.(TxnInitArgs)
	var txnInitRet = TxnInitRet{TxnInitArgs: txnInitArgs}
	v := txnInitArgs.A * txnInitArgs.B
	if v >= 0 {
		txnInitRet.Threshold, errCode = v, 0
	} else {
		txnInitRet.Threshold, errCode = -1, 1
	}
	ret = txnInitRet
	return
}

var keyHashFunc KeyHashFunc = func(key string) uint64 {
	return uint64(key[0])
}
