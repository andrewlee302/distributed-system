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
func registerStruct() {
	gob.Register(SetArgs{})
	gob.Register(AddArgs{})
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
		ppts[i] = NewParticipant(pptsPorts, i, coordPort)
		ppts[i].RegisterCaller(CallFunc(dbs[i].Add), "Add")
		ppts[i].RegisterCaller(CallFunc(dbs[i].Set), "Set")
	}
	coord = NewCoordinator(pptsPorts, coordPort)

	fmt.Printf("Test: Basic")

	txn := coord.NewTxn(initFunc, keyHashFunc, 10000)

	registerStruct()
	txn.SetTxnPart("1", "Set", SetArgs{Idx: 1, SetValue: 2})
	txn.SetTxnPart("2", "Add", AddArgs{Idx: 2, Delta: 3})
	coord.StartTxn(txn, 0)

	for coord.StateTxn(txn.ID) != StateTxnCommitted {
	}
	checkDB(t, dbs[keyHashFunc("1")%nppt], 1, 2)
	checkDB(t, dbs[keyHashFunc("2")%nppt], 2, 3)
	cleanup(ppts, coord)
}

func checkDB(t *testing.T, db *BasicDataBase, idx int, wanted int) {
	v := db.dataset[idx]
	if v != wanted {
		t.Fatalf("db.dataset[%d]; value=%v wanted=%v", idx, v, wanted)
	}
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
		ppts[i] = NewParticipant(pptsPorts, i, coordPort)
		ppts[i].RegisterCaller(CallFunc(dbs[i].Add), "Add")
		ppts[i].RegisterCaller(CallFunc(dbs[i].Set), "Set")
	}
	coord = NewCoordinator(pptsPorts, coordPort)

	fmt.Printf("Test: Basic")

	txn := coord.NewTxn(initFunc, keyHashFunc, 10000)

	txn.SetTxnPart("1", "Set", SetArgs{Idx: 1, SetValue: 2})
	txn.SetTxnPart("2", "Add", AddArgs{Idx: 2, Delta: 3})
	coord.StartTxn(txn, 1)

	for coord.StateTxn(txn.ID) != StateTxnAborted {
	}
	checkDB(t, dbs[keyHashFunc("1")%nppt], 1, 0)
	checkDB(t, dbs[keyHashFunc("2")%nppt], 2, 0)

	cleanup(ppts, coord)
}

type BasicDataBase struct {
	dataset map[int]int
}

type SetArgs struct {
	Idx, SetValue int
}

type AddArgs struct {
	Idx, Delta int
}

func (base *BasicDataBase) Set(args interface{}, initRet interface{}) (errCode int, rbf Rollbacker) {
	setArgs := args.(SetArgs)
	origin := base.dataset[setArgs.Idx]
	rbf = RollbackFunc(func() {
		base.dataset[setArgs.Idx] = origin
	})
	base.dataset[setArgs.Idx] = setArgs.SetValue
	errCode = 0
	return
}

// If base.dataset[txnArgs.Idx] is less than threshold, then failed, when errCode = 1.
func (base *BasicDataBase) Add(args interface{}, thresholdInt interface{}) (errCode int, rbf Rollbacker) {
	addArgs := args.(AddArgs)
	threshold := thresholdInt.(int)
	origin := base.dataset[addArgs.Idx]
	rbf = RollbackFunc(func() {
		base.dataset[addArgs.Idx] = origin
	})
	if base.dataset[addArgs.Idx] < threshold {
		errCode = 1
	} else {
		errCode = 0
		base.dataset[addArgs.Idx] += addArgs.Delta
	}
	return
}

// Multiple the param by 2. If it's not a number ,then failed, when errCode = 1.
var initFunc TxnInitFunc = func(args interface{}) (ret interface{}, errCode int) {
	if v, ok := args.(int); !ok {
		ret, errCode = nil, 1
	} else {
		ret, errCode = v*2, 0
	}
	return
}

var keyHashFunc KeyHashFunc = func(key string) uint64 {
	return uint64(key[0])
}
