package shopping

import (
	"distributed-system/kv"
	"distributed-system/tinykv"
	"distributed-system/twopc"
	"encoding/gob"
	"sync"
)

// ShoppingTxnKVStore supports the shopping transations. It wraps
// the KV-store with the shopping txn logics. The logics will be registered
// into the 2pc service, so the txns could be managed by the Coordinator
// and the Parcipants.
type ShoppingTxnKVStore struct {
	kv.KVStoreService
	transRwLock sync.RWMutex
}

// NewShoppingTxnKVStore inits a new ShoppingTxnKVStore.
func NewShoppingTxnKVStore() *ShoppingTxnKVStore {
	tks := &ShoppingTxnKVStore{KVStoreService: tinykv.NewKVStore()}
	return tks
}

// ShoppingTxnKVStoreService is the service wrapping the ShoppingTxnKVStore
// and one partipant of 2pc.
type ShoppingTxnKVStoreService struct {
	*ShoppingTxnKVStore
	ppt     *twopc.Participant
	network string
}

// NewShoppingTxnKVStoreService starts the transaction-enabled kvstore.
func NewShoppingTxnKVStoreService(network, addr, coordAddr string) *ShoppingTxnKVStoreService {
	ppt := twopc.NewParticipant(network, addr, coordAddr)
	if ppt == nil {
		return nil
	}
	sks := NewShoppingTxnKVStore()

	gob.Register(AddItemTxnInitRet{})
	ppt.RegisterCaller(twopc.CallFunc(sks.CartExist), "CartExist")
	ppt.RegisterCaller(twopc.CallFunc(sks.CartAddItem), "CartAddItem")

	gob.Register(SubmitOrderTxnInitRet{})
	ppt.RegisterCaller(twopc.CallFunc(sks.ItemsStockMinus), "ItemsStockMinus")
	ppt.RegisterCaller(twopc.CallFunc(sks.OrderRecord), "OrderRecord")

	gob.Register(PayOrderTxnInitRet{})
	ppt.RegisterCaller(twopc.CallFunc(sks.PayMinus), "PayMinus")
	ppt.RegisterCaller(twopc.CallFunc(sks.PayAdd), "PayAdd")
	ppt.RegisterCaller(twopc.CallFunc(sks.PayRecord), "PayRecord")
	service := &ShoppingTxnKVStoreService{ShoppingTxnKVStore: sks, network: network, ppt: ppt}
	ppt.RegisterRPCService(service)
	return service
}

// Serve starts the KV-store service.
func (service *ShoppingTxnKVStoreService) Serve() {
	// ?
}
