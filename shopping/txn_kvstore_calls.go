package shopping

import (
	"distributed-system/twopc"
	"fmt"
	"strconv"
)

// CartExist is the subruntine as a part of txn, to check whether the
// cart exists.
func (skv *ShoppingTxnKVStore) CartExist(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	// fmt.Println("CartExist start", initRet)
	// defer fmt.Println("CartExist end", errCode)
	args := initRet.(AddItemTxnInitRet)
	rbf = twopc.BlankRollbackFunc

	var value string
	var existed bool

	cartID, _ := strconv.Atoi(args.CartIDStr)
	// Test whether the cart exists,
	var maxCartID = 0
	if value, existed = skv.Get(CartIDMaxKey); existed {
		maxCartID, _ = strconv.Atoi(value)
		if cartID > maxCartID || cartID < 1 {
			errCode = TxnNotFound
			return
		}
	} else {
		errCode = TxnNotFound
		return
	}
	errCode = TxnOK
	return
}

// CartAddItem is the subruntine as a part of txn, to add or discard some
// items to a specific cart.
func (skv *ShoppingTxnKVStore) CartAddItem(initRet interface{}) (errCode int,
	rbf twopc.Rollbacker) {
	// fmt.Println("CartAuthAndValid start:", initRet)
	// defer func() { fmt.Println("CartAuthAndValid end:", errCode) }()

	args := initRet.(AddItemTxnInitRet)
	rbf = twopc.BlankRollbackFunc

	var value string
	var existed bool

	// Test whether the cart belongs other users.
	if value, existed = skv.Get(args.CartKey); !existed {
		errCode = TxnNotAuth
		return
	}
	num, cartDetail := parseCartValue(value)

	// Test whether #items in cart exceeds 3.
	if num+args.AddItemCnt > 3 {
		errCode = TxnItemOutOfLimit
		return
	}

	num += args.AddItemCnt
	// Set the new values of the cart.
	cartDetail[args.ItemID] += args.AddItemCnt
	skv.Put(args.CartKey, composeCartValue(num, cartDetail))
	rbf = twopc.RollbackFunc(func() {
		skv.Put(args.CartKey, value)
	})

	errCode = TxnOK
	return
}

// ItemsStockMinus is the subruntine as a part of txn, to minus the stock of
// one item when a order is made.
//
// The subruntine is executed in a broadcast way, i.e. all the shards will
// execute it.
func (skv *ShoppingTxnKVStore) ItemsStockMinus(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	// fmt.Println("ItemsStockMinus start:", initRet)
	// defer func() { fmt.Println("ItemsStockMinus end:", errCode) }()
	args := initRet.(SubmitOrderTxnInitRet)
	_, cartDetail := parseCartValue(args.CartValue)
	errCode = TxnOK
	for itemID, itemCnt := range cartDetail {
		itemsStockKey := ItemsStockKeyPrefix + strconv.Itoa(itemID)
		// Must check whether the key does exist or not.
		if _, existed := skv.Get(itemsStockKey); existed {
			newValue, _, _ := skv.Incr(itemsStockKey, 0-itemCnt)
			iNewValue, _ := strconv.Atoi(newValue)
			if iNewValue < 0 {
				errCode = TxnOutOfStock
			}
		}
	}

	rbf = twopc.RollbackFunc(func() {
		for itemID, itemCnt := range cartDetail {
			itemsStockKey := ItemsStockKeyPrefix + strconv.Itoa(itemID)
			if _, existed := skv.Get(itemsStockKey); existed {
				skv.Incr(itemsStockKey, itemCnt)
			}
		}
	})

	return
}

// OrderRecord is the subruntine as a part of txn, to record the order when a
// order is made.
func (skv *ShoppingTxnKVStore) OrderRecord(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	// fmt.Println("OrderRecord start:", initRet)
	// defer func() { fmt.Println("OrderRecord end:", errCode) }()
	args := initRet.(SubmitOrderTxnInitRet)
	num, cartDetail := parseCartValue(args.CartValue)

	// Record the order and delete the cart.
	oldValue, existed := skv.Put(args.OrderKey, composeOrderValue(false, args.Price, num, cartDetail))
	if existed {
		errCode = TxnOrderOutOfLimit
		rbf = twopc.RollbackFunc(func() {
			skv.Put(args.OrderKey, oldValue)
		})
		return
	}
	rbf = twopc.RollbackFunc(func() {
		skv.Del(args.OrderKey)
	})
	errCode = TxnOK
	return
}

// PayMinus is the subruntine as a part of txn, to decrease the user's balance
// when a order is paid.
func (skv *ShoppingTxnKVStore) PayMinus(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	fmt.Println("PayMinus start:", initRet)
	defer func() { fmt.Println("PayMinus end:", errCode) }()
	args := initRet.(PayOrderTxnInitRet)
	rbf = twopc.BlankRollbackFunc

	rbf = twopc.RollbackFunc(func() {
		skv.Incr(args.BalanceKey, args.Delta)
	})
	// Decrease the balance of the user.
	newVal, _, _ := skv.Incr(args.BalanceKey, 0-args.Delta)
	iNewVal, _ := strconv.Atoi(newVal)
	if iNewVal < 0 {
		errCode = TxnBalanceInsufficient
		return
	}
	errCode = TxnOK
	return
}

// PayAdd is the subruntine as a part of txn, to increase the root's balance
// when a order is paid.
func (skv *ShoppingTxnKVStore) PayAdd(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	fmt.Println("PayAdd start:", initRet)
	defer func() { fmt.Println("PayAdd end:", errCode) }()
	args := initRet.(PayOrderTxnInitRet)
	rbf = twopc.RollbackFunc(func() {
		skv.Incr(args.RootBalanceKey, 0-args.Delta)
	})
	skv.Incr(args.RootBalanceKey, args.Delta)
	errCode = TxnOK
	return
}

// PayRecord is the subruntine as a part of txn, to record the payment
// when a order is paid.
func (skv *ShoppingTxnKVStore) PayRecord(initRet interface{}) (errCode int, rbf twopc.Rollbacker) {
	fmt.Println("PayRecord start:", initRet)
	defer func() { fmt.Println("PayRecord end:", errCode) }()
	args := initRet.(PayOrderTxnInitRet)

	// must exist
	orderValue, _ := skv.Get(args.OrderKey)

	hasPaid, price, num, detail := parseOrderValue(orderValue)

	if hasPaid {
		rbf = twopc.BlankRollbackFunc
		errCode = TxnOrderPaid
		return
	}

	newOrderValue := composeOrderValue(true, price, num, detail)
	oldValue, _ := skv.Put(args.OrderKey, newOrderValue)

	rbf = twopc.RollbackFunc(func() {
		skv.Put(args.OrderKey, oldValue)
	})

	errCode = TxnOK
	return
}
