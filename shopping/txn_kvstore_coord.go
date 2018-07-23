package shopping

import (
	"distributed-system/twopc"
	"distributed-system/util"
	"encoding/gob"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

// ShoppingTxnCoordinator wraps the Coordinator with the RPC services to
// serve the txn logics.
type ShoppingTxnCoordinator struct {
	coord       *twopc.Coordinator
	itemList    []Item // real item start from index 1
	hub         *ShardsClientHub
	keyHashFunc twopc.KeyHashFunc
	timeoutMs   int64

	tasks chan *TxnTask
}

// TxnTask records a txn and its initial args and return code.
type TxnTask struct {
	txn      *twopc.Txn
	initArgs interface{}
	errCode  int
}

// DefaultTaskMaxSize is the maximum of TxnTasks waiting to be processed
// one by one.
const DefaultTaskMaxSize = 10000

// NewShoppingTxnCoordinator inits ShoppingTxnCoordinator service.
func NewShoppingTxnCoordinator(network, coord string, ppts []string,
	keyHashFunc twopc.KeyHashFunc, timeoutMs int64) *ShoppingTxnCoordinator {
	coordS := twopc.NewCoordinator(network, coord, ppts)
	if coordS == nil {
		return nil
	}
	sts := &ShoppingTxnCoordinator{coord: coordS,
		keyHashFunc: keyHashFunc, timeoutMs: timeoutMs,
		hub:   NewShardsClientHub(network, ppts, keyHashFunc, 1),
		tasks: make(chan *TxnTask, DefaultTaskMaxSize)}
	go func() {
		for _ = range time.Tick(time.Second * 5) {
			ns := atomic.LoadInt64(&util.RPCCallNs)
			fmt.Printf("RPCCall cost %v ms\n", ns/time.Millisecond.Nanoseconds())
		}
	}()
	sts.coord.RegisterService(sts)
	gob.Register(AddItemTxnInitRet{})
	gob.Register(SubmitOrderTxnInitRet{})
	gob.Register(PayOrderTxnInitRet{})
	go sts.Run()
	return sts
}

// LoadItemList loads item info into the cahce for the slater rapid visiting.
func (stc *ShoppingTxnCoordinator) LoadItemList(itemsSize *int, reply *struct{}) error {
	stc.itemList = make([]Item, 1+*itemsSize)
	for itemID := 1; itemID <= *itemsSize; itemID++ {
		_, reply := stc.hub.Get(ItemsPriceKeyPrefix + strconv.Itoa(itemID))
		price, _ := strconv.Atoi(reply.Value)

		_, reply = stc.hub.Get(ItemsStockKeyPrefix + strconv.Itoa(itemID))
		stock, _ := strconv.Atoi(reply.Value)

		stc.itemList[itemID] = Item{ID: itemID, Price: price, Stock: stock}
	}
	return nil
}

// AddItemArgs is the argument of the AddItemTrans function.
type AddItemArgs struct {
	CartIDStr  string
	UserToken  string
	ItemID     int
	AddItemCnt int
}

// AddItemTxnInitArgs is intial args for AddItem txn's initialization.
type AddItemTxnInitArgs struct {
	OrderKey   string
	CartKey    string
	CartIDStr  string
	ItemID     int
	AddItemCnt int
}

// AddItemTxnInitRet is return value for AddItem txn's initialization.
type AddItemTxnInitRet AddItemTxnInitArgs

// AddItemTxnInit is the initial function for AddItem txn.
func AddItemTxnInit(args interface{}) (ret interface{}, errCode int) {
	initArgs := args.(*AddItemTxnInitArgs)
	ret = AddItemTxnInitRet(*initArgs)
	errCode = 0
	return
}

// AsyncAddItemTxn submits the AddItem txn to the tasks list, and then
// returns immediately.
func (stc *ShoppingTxnCoordinator) AsyncAddItemTxn(args *AddItemArgs, txnID *string) error {
	cartKey := getCartKey(args.CartIDStr, args.UserToken)
	orderKey := OrderKeyPrefix + args.UserToken

	txn := stc.coord.NewTxn(AddItemTxnInit, stc.keyHashFunc, stc.timeoutMs)
	*txnID = txn.ID

	txn.AddTxnPart(CartIDMaxKey, "CartExist")

	txn.AddTxnPart(orderKey, "CartOrdered")

	txn.AddTxnPart(cartKey, "CartAddItem")

	initArgs := &AddItemTxnInitArgs{OrderKey: orderKey,
		CartKey:   cartKey,
		CartIDStr: args.CartIDStr, ItemID: args.ItemID,
		AddItemCnt: args.AddItemCnt}
	// fmt.Println("AsyncAddItemTxn", initArgs)
	stc.tasks <- &TxnTask{txn: txn, initArgs: initArgs}
	return nil
}

// Run executes the txns one by one from the task list.
func (stc *ShoppingTxnCoordinator) Run() {
	// TODO: how to exploit the potential of parallelism.
	for task := range stc.tasks {
		task.txn.Start(task.initArgs)
		var reply twopc.TxnState
		stc.coord.SyncTxnEnd(&task.txn.ID, &reply)
	}
}

// SubmitOrderArgs is the args for the SubmitOrder txn.
type SubmitOrderArgs struct {
	CartIDStr string
	UserToken string
}

// SubmitOrderTxnInitArgs is intial args for SubmitOrder txn's initialization.
type SubmitOrderTxnInitArgs struct {
	stc       *ShoppingTxnCoordinator
	hub       *ShardsClientHub
	OrderKey  string
	CartIDStr string
	CartKey   string
}

// SubmitOrderTxnInitRet is return value for SubmitOrder txn's initialization.
type SubmitOrderTxnInitRet struct {
	SubmitOrderTxnInitArgs
	CartValue string
	Price     int
}

// SubmitOrderTxnInit is the initial function for SubmitOrder txn.
func SubmitOrderTxnInit(args interface{}) (ret interface{}, errCode int) {
	initArgs := args.(*SubmitOrderTxnInitArgs)

	ok, reply := initArgs.hub.Get(initArgs.CartKey)
	if !ok {
		errCode = -3
		return
	}
	cartValue := reply.Value
	price := 0
	if cartValue != "" {
		_, cartDetail := parseCartValue(cartValue)
		for itemID, itemCnt := range cartDetail {
			price += itemCnt * initArgs.stc.itemList[itemID].Price
		}
	}
	// if cartValue == 0, we don't know it's TxnNotFound or TxnNotAuth,
	// so we move on. In two cases, make sure there is no runtime errors
	// in ItemsStockMinus and OrderRecord .

	ret = &SubmitOrderTxnInitRet{SubmitOrderTxnInitArgs: *initArgs,
		CartValue: cartValue, Price: price}
	errCode = 0
	return
}

// AsyncSubmitOrderTxn submits SubmitOrder txn to the task list.
func (stc *ShoppingTxnCoordinator) AsyncSubmitOrderTxn(args *SubmitOrderArgs, txnID *string) error {
	cartKey := getCartKey(args.CartIDStr, args.UserToken)
	orderKey := OrderKeyPrefix + args.UserToken

	txn := stc.coord.NewTxn(SubmitOrderTxnInit, stc.keyHashFunc, stc.timeoutMs)
	*txnID = txn.ID

	txn.BroadcastTxnPart("ItemsStockMinus")

	txn.AddTxnPart(orderKey, "OrderRecord")

	initArgs := &SubmitOrderTxnInitArgs{stc: stc, hub: stc.hub,
		CartIDStr: args.CartIDStr, CartKey: cartKey,
		OrderKey: orderKey}

	// fmt.Println("AsyncSubmitOrderTxn", initArgs)
	stc.tasks <- &TxnTask{txn: txn, initArgs: initArgs}
	return nil
}

// PayOrderArgs is the args for PayOrder txn.
type PayOrderArgs struct {
	OrderIDStr string
	UserToken  string
	Delta      int
}

// PayOrderTxnInitArgs is initial args for PayOrder txn's initialization.
type PayOrderTxnInitArgs struct {
	hub            *ShardsClientHub
	OrderKey       string
	BalanceKey     string
	RootBalanceKey string
	Delta          int
}

// PayOrderTxnInitRet is return value for PayOrder txn's initialization.
type PayOrderTxnInitRet PayOrderTxnInitArgs

// PayOrderTxnInit is the initial function for PayOrder txn.
func PayOrderTxnInit(args interface{}) (ret interface{}, errCode int) {
	initArgs := args.(*PayOrderTxnInitArgs)
	ret = PayOrderTxnInitRet(*initArgs)
	errCode = 0
	return
}

// AsyncPayOrderTxn submits PayOrder txn to the task list.
func (stc *ShoppingTxnCoordinator) AsyncPayOrderTxn(args *PayOrderArgs, txnID *string) error {
	balanceKey := BalanceKeyPrefix + args.UserToken
	rootBalanceKey := BalanceKeyPrefix + RootUserToken
	orderKey := OrderKeyPrefix + args.OrderIDStr

	txn := stc.coord.NewTxn(PayOrderTxnInit, stc.keyHashFunc, stc.timeoutMs)
	*txnID = txn.ID

	txn.AddTxnPart(balanceKey, "PayMinus")

	txn.AddTxnPart(rootBalanceKey, "PayAdd")

	txn.AddTxnPart(orderKey, "PayRecord")

	initArgs := &PayOrderTxnInitArgs{hub: stc.hub,
		BalanceKey: balanceKey, RootBalanceKey: rootBalanceKey,
		OrderKey: orderKey, Delta: args.Delta}

	fmt.Println("AsyncPayOrderTxn", initArgs)
	stc.tasks <- &TxnTask{txn: txn, initArgs: initArgs}
	return nil
}
