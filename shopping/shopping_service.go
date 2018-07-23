/*
Package shopping provides rush shopping backend service.

Requirements

The RESTful API is identified in spec.md, including the URL, logic semantics,
request format and response status.

Some important constrains of rush shopping are highlighted as follows.
 * Every item has a limited stock and can't be oversold. The stock constraint
 should be guaranteed when the order is made instead of managing the cart.
 * Every user can create more than one cart, but the total of items in it must
 not more than three.
 * Every user can't make more than one order.

The normal actions for a user to finish purchasing the order are as follows.

                                  | /carts/xxx |
 /login --- /items --- /carts --- | /carts/xxx | --- /orders --- /pay
                                  | /carts/xxx |

Actually, a user could create carts before querying the items. Of course,
a user could add items to cart or discard items by /carts/xxx in any times.
Just make sure the total of items in the cart must not more than three.

The admin username and password are as follows.
 | admin | username | password |
 | value | root     | root     |

And the username with the prefix "zero" means the corresponding the user's
balance is zero, so she/he can't afford any item.

The CSV format for the users is the following.
 | id  | username | password | balance |
 | int | string   | string   | int     |

The CSV format for the items is the following.
 | id  | price | stock |
 | int | int   | int   |


An implementation in our project

We assume the followings:
 * The IDs of items are increasing from 1 continuously.
 * The ID of the (root) administrator user is 0.
 * The IDs of normal users are increasing from 1 continuously.
 * CartID is auto-increased from 1.

 The data format in KV-Store could be referred in shop_kvformat.md.
*/
package shopping

import (
	"distributed-system/twopc"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"rush-shopping/kv"
	"strconv"
	"strings"
	"sync"
	"time"
)

// API URLs
const (
	URLLogin              = "/login"
	URLQueryItem          = "/items"
	URLCreateCart         = "/carts"
	URLAddItem            = "/carts/"
	URLSubmitOrQueryOrder = "/orders"
	URLPayOrder           = "/pay"
	URLQueryAllOrders     = "/admin/orders"
)

// Keys of kvstore.
const (
	TokenKeyPrefix      = "token:"
	OrderKeyPrefix      = "order:"
	ItemsStockKeyPrefix = "items_stock:"
	ItemsPriceKeyPrefix = "items_price:"
	BalanceKeyPrefix    = "balance:"

	CartIDMaxKey = "cartID"
	ItemsSizeKey = "items_size"
)

// Flags for paid or unpaid status.
const (
	OrderPaidFlag   = "P" // have been paid
	OrderUnpaidFlag = "W" // wait to be paid
)

// RootUserID is specified.
const RootUserID = 0

// RootUserToken is token for the root user.
var RootUserToken = userID2Token(RootUserID)

// Trans status.
const (
	TxnOK       = 0
	TxnNotFound = 1 << (iota - 1) // iota == 1
	TxnNotAuth
	TxnCartEmpyt
	TxnOutOfStock      // out of stock
	TxnItemOutOfLimit  // most 3 items
	TxnOrderOutOfLimit // most one order for one person
	TxnOrderPaid
	TxnBalanceInsufficient
)

// JSON-format msgs for requests.
var (
	UserAuthFailMsg       = []byte("{\"code\":\"USER_AUTH_FAIL\",\"message\":\"用户名或密码错误\"}")
	MalformedJSONMsg      = []byte("{\"code\": \"MALFORMED_JSON\",\"message\": \"格式错误\"}")
	EmptyRequestMsg       = []byte("{\"code\": \"EMPTY_REQUEST\",\"message\": \"请求体为空\"}")
	InvalidAccessTokenMsg = []byte("{\"code\": \"INVALID_ACCESS_TOKEN\",\"message\": \"无效的令牌\"}")
	CartNotFoundMsg       = []byte("{\"code\": \"CART_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	CartEmptyMsg          = []byte("{\"code\": \"CART_EMPTY\", \"message\": \"购物车为空\"}")
	NotAuthorizedCartMsg  = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_CART\",\"message\": \"无权限访问指定的篮子\"}")
	ItemOutOfLimitMsg     = []byte("{\"code\": \"ITEM_OUT_OF_LIMIT\",\"message\": \"篮子中物品数量超过了三个\"}")
	ItemNotFoundMsg       = []byte("{\"code\": \"ITEM_NOT_FOUND\",\"message\": \"物品不存在\"}")
	ItemOutOfStockMsg     = []byte("{\"code\": \"ITEM_OUT_OF_STOCK\", \"message\": \"物品库存不足\"}")
	OrderOutOfLimitMsg    = []byte("{\"code\": \"ORDER_OUT_OF_LIMIT\",\"message\": \"每个用户只能下一单\"}")

	OrderNotFoundMsg       = []byte("{\"code\": \"ORDER_NOT_FOUND\", \"message\": \"篮子不存在\"}")
	NotAuthorizedOrderMsg  = []byte("{\"code\": \"NOT_AUTHORIZED_TO_ACCESS_ORDER\",\"message\": \"无权限访问指定的订单\"}")
	OrderPaidMsg           = []byte("{\"code\": \"ORDER_PAID\",\"message\": \"订单已支付\"}")
	BalanceInsufficientMsg = []byte("{\"code\": \"BALANCE_INSUFFICIENT\",\"message\": \"余额不足\"}")
)

// ShopServer is the web service for rush shopping.
type ShopServer struct {
	server    *http.Server
	handler   *http.Handler
	rootToken string

	// coordClients is clients to the Coordinator in 2pc. It works in
	// transactions.
	coordClients *CoordClients

	// clientHub is the clients directly to the KV-stores. It will distribute
	// different request to the specific shard of the whole KV-store.
	clientHub *ShardsClientHub

	// All the followings are in the resident memory.
	// Item start from index 1.
	ItemListCache []Item
	ItemLock      sync.Mutex

	// ItemsJSONCache is cache for querying items.
	ItemsJSONCache []byte

	// UserMap is the users' information.
	UserMap map[string]UserIDAndPass

	// MaxItemID is the same with the number of types of items.
	MaxItemID int

	// MaxUserID is the same with the number of normal users.
	MaxUserID int
}

// DefaultShardClientPoolMaxSize is the default size of connection pool for
// every shard of the whole KV-store.
const DefaultShardClientPoolMaxSize = 100

// DefaultCoordClientPoolMaxSize is the default size of connection pool for
// the Coordinator in 2pc.
const DefaultCoordClientPoolMaxSize = 100

// InitService inits the rush shopping web service and starts the service.
// Network is "tcp" or "unix" for the whole system. CoordAddr and kvstoreAddrs
// communication addresses for the Coordinator and shards. UserCsv
// and itemCsv is the CSV file path for users and items. KeyHashFunc is the
// hash function to distribute the key-value pair to the specific shard.
func InitService(network, appAddr, coordAddr, userCsv, itemCsv string,
	kvstoreAddrs []string, keyHashFunc twopc.KeyHashFunc) *ShopServer {
	ss := new(ShopServer)
	ss.coordClients = NewCoordClients(network, coordAddr, DefaultCoordClientPoolMaxSize)
	ss.clientHub = NewShardsClientHub(network, kvstoreAddrs, keyHashFunc, DefaultShardClientPoolMaxSize)
	ss.loadUsersAndItems(userCsv, itemCsv)

	handler := http.NewServeMux()
	ss.server = &http.Server{
		Addr:    appAddr,
		Handler: handler,
		// ReadTimeout:    10 * time.Second,
		// WriteTimeout:   10 * time.Second,
		// MaxHeaderBytes: 1 << 20,
	}
	handler.HandleFunc(URLLogin, ss.login)
	handler.HandleFunc(URLQueryItem, ss.queryItem)
	handler.HandleFunc(URLCreateCart, ss.createCart)
	handler.HandleFunc(URLAddItem, ss.addItem)
	handler.HandleFunc(URLSubmitOrQueryOrder, ss.orderProcess)
	handler.HandleFunc(URLPayOrder, ss.payOrder)
	// handler.HandleFunc(QUERY_ALL_ORDERS, ss.queryAllOrders)
	log.Printf("Start shopping service on %s\n", appAddr)
	go func() {
		if err := ss.server.ListenAndServe(); err != nil {
			fmt.Println(err)
		}
	}()
	return ss
}

// Kill kills the shopping service for a test.
func (ss *ShopServer) Kill() {
	log.Println("Kill the http server")
	if err := ss.server.Close(); err != nil {
		log.Fatal("Http server close error:", err)
	}
}

// Load user and item data to KV-stores.
func (ss *ShopServer) loadUsersAndItems(userCsv, itemCsv string) {
	log.Println("Load user and item data to kvstore")
	now := time.Now()
	defer func() {
		log.Printf("Finished data loading, cost %v ms\n", time.Since(now).Nanoseconds()/int64(time.Millisecond))
	}()

	ss.clientHub.Put(CartIDMaxKey, "0")

	ss.ItemListCache = make([]Item, 1, 512)
	ss.ItemListCache[0] = Item{ID: 0}

	ss.UserMap = make(map[string]UserIDAndPass)

	var wg sync.WaitGroup

	// read users
	if file, err := os.Open(userCsv); err == nil {
		reader := csv.NewReader(file)
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			wg.Add(1)
			userID, _ := strconv.Atoi(strs[0])
			ss.UserMap[strs[1]] = UserIDAndPass{userID, strs[2]}
			userToken := userID2Token(userID)
			go func(token, value string) {
				ss.clientHub.Put(BalanceKeyPrefix+token, value)
				wg.Done()
			}(userToken, strs[3])
			if userID > ss.MaxUserID {
				ss.MaxUserID = userID
			}
		}
		file.Close()
	} else {
		panic(err.Error())
	}

	ss.rootToken = userID2Token(ss.UserMap["root"].ID)

	// read items
	itemCnt := 0
	if file, err := os.Open(itemCsv); err == nil {
		reader := csv.NewReader(file)
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			itemCnt++
			itemID, _ := strconv.Atoi(strs[0])
			price, _ := strconv.Atoi(strs[1])
			stock, _ := strconv.Atoi(strs[2])
			ss.ItemListCache = append(ss.ItemListCache, Item{ID: itemID, Price: price, Stock: stock})

			wg.Add(2)
			go func(key, value string) {
				ss.clientHub.Put(ItemsPriceKeyPrefix+key, value)
				wg.Done()
			}(strs[0], strs[1])
			go func(key, value string) {
				ss.clientHub.Put(ItemsStockKeyPrefix+key, value)
				wg.Done()
			}(strs[0], strs[2])

			if itemID > ss.MaxItemID {
				ss.MaxItemID = itemID
			}
		}
		ss.ItemsJSONCache, _ = json.Marshal(ss.ItemListCache[1:])
		wg.Wait()
		ss.clientHub.Put(ItemsSizeKey, strconv.Itoa(itemCnt))

		file.Close()
	} else {
		panic(err.Error())
	}
	ss.coordClients.LoadItemList(itemCnt)
}

func (ss *ShopServer) login(writer http.ResponseWriter, req *http.Request) {
	isEmpty, body := isBodyEmpty(writer, req)
	if isEmpty {
		return
	}
	var user LoginJson
	if err := json.Unmarshal(body, &user); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MalformedJSONMsg)
		return
	}
	userIDAndPass, ok := ss.UserMap[user.Username]
	if !ok || userIDAndPass.Password != user.Password {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(UserAuthFailMsg)
		return
	}

	userID := userIDAndPass.ID
	token := userID2Token(userID)
	ss.clientHub.Put(TokenKeyPrefix+token, "1")
	okMsg := []byte("{\"user_id\":" + strconv.Itoa(userID) + ",\"username\":\"" +
		user.Username + "\",\"access_token\":\"" + token + "\"}")
	writer.WriteHeader(http.StatusOK)
	writer.Write(okMsg)
}

// TODO consistency tradeoff for perf
func (ss *ShopServer) queryItem(writer http.ResponseWriter, req *http.Request) {
	if exist, _ := ss.authorize(writer, req, ss.clientHub, false); !exist {
		return
	}
	// var wg sync.WaitGroup
	// wg.Add(len(ss.ItemListCache) - 1)
	// // TODO data race
	// ss.ItemLock.Lock()
	// for i := 1; i < len(ss.ItemListCache); i++ {
	// 	go func(i int) {
	// 		_, reply := ss.clientHub.Get(ItemsPriceKeyPrefix + strconv.Itoa(ss.ItemList[i].ID))
	// 		ss.ItemListCache[i].Stock, _ = strconv.Atoi(reply.Value)
	// 		wg.Done()
	// 	}(i)
	// }
	// wg.Wait()
	// ss.ItemsJSONCache, _ = json.Marshal(ss.ItemListCache[1:])
	// ss.ItemLock.Unlock()

	writer.WriteHeader(http.StatusOK)
	writer.Write(ss.ItemsJSONCache)
	return
}

func (ss *ShopServer) createCart(writer http.ResponseWriter, req *http.Request) {

	var token string
	exist, token := ss.authorize(writer, req, ss.clientHub, false)
	if !exist {
		return
	}

	_, reply := ss.clientHub.Incr(CartIDMaxKey, 1)
	cartIDStr := reply.Value

	cartKey := getCartKey(cartIDStr, token)
	_, reply = ss.clientHub.Put(cartKey, "0")

	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("{\"cart_id\": \"" + cartIDStr + "\"}"))
	return
}

func (ss *ShopServer) addItem(writer http.ResponseWriter, req *http.Request) {
	var token string
	exist, token := ss.authorize(writer, req, ss.clientHub, false)
	if !exist {
		return
	}

	isEmpty, body := isBodyEmpty(writer, req)
	if isEmpty {
		return
	}

	var item ItemCount
	if err := json.Unmarshal(body, &item); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MalformedJSONMsg)
		return
	}

	if item.ItemID < 1 || item.ItemID > ss.MaxItemID {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(ItemNotFoundMsg)
		return
	}

	cartIDStr := strings.Split(req.URL.Path, "/")[2]
	cartKey := getCartKey(cartIDStr, token)
	existed, cartValue := ss.checkCartExist(cartIDStr, cartKey, writer, req)
	if !existed {
		return
	}
	num, cartDetail := parseCartValue(cartValue)

	// Test whether #items in cart exceeds 3.
	if num+item.Count > 3 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(ItemOutOfLimitMsg)
		return
	}

	num += item.Count
	// Set the new values of the cart.
	cartDetail[item.ItemID] += item.Count
	ss.clientHub.Put(cartKey, composeCartValue(num, cartDetail))
	writer.WriteHeader(http.StatusNoContent)
	return
}

func (ss *ShopServer) orderProcess(writer http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		ss.submitOrder(writer, req)
		// fmt.Println("submitOrder")
	} else {
		ss.queryOneOrder(writer, req)
		// fmt.Println("queryOneOrder")
	}
}

func (ss *ShopServer) submitOrder(writer http.ResponseWriter, req *http.Request) {

	var token string
	exist, token := ss.authorize(writer, req, ss.clientHub, false)
	if !exist {
		return
	}

	isEmpty, body := isBodyEmpty(writer, req)
	if isEmpty {
		return
	}
	var cartIDJson CartIDJson
	if err := json.Unmarshal(body, &cartIDJson); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MalformedJSONMsg)
		return
	}
	cartIDStr := cartIDJson.IDStr
	cartKey := getCartKey(cartIDStr, token)
	existed, cartValue := ss.checkCartExist(cartIDStr, cartKey, writer, req)
	if !existed {
		return
	}

	// Test whether the cart is empty.
	num, _ := parseCartValue(cartValue)
	if num == 0 {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(CartEmptyMsg)
		return
	}

	// ss.coordClients.AsyncSubmitOrderTxn(cartIDStr, token)

	// writer.WriteHeader(http.StatusOK)
	// writer.Write([]byte("{\"order_id\": \"" + token + "\"}"))
	// return

	_, txnID := ss.coordClients.AsyncSubmitOrderTxn(cartIDStr, token)
	errCode := ss.coordClients.SyncTxn(txnID)
	flag := normalizeErrCode(errCode)
	// fmt.Println("submitOrder", flag)
	switch flag {
	case TxnOK:
		{
			writer.WriteHeader(http.StatusOK)
			writer.Write([]byte("{\"order_id\": \"" + token + "\"}"))
		}
	case TxnOutOfStock:
		{
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(ItemOutOfStockMsg)
		}
	case TxnOrderOutOfLimit:
		{
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(OrderOutOfLimitMsg)
		}
	}
	return
}

func (ss *ShopServer) payOrder(writer http.ResponseWriter, req *http.Request) {
	var token string

	exist, token := ss.authorize(writer, req, ss.clientHub, false)
	if !exist {
		return
	}

	isEmpty, body := isBodyEmpty(writer, req)
	if isEmpty {
		return
	}
	var orderIDJson OrderIDJson
	if err := json.Unmarshal(body, &orderIDJson); err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(MalformedJSONMsg)
		return
	}

	orderIDStr := orderIDJson.IDStr
	if orderIDStr != token {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NotAuthorizedOrderMsg)
		return
	}

	orderKey := OrderKeyPrefix + orderIDStr
	// Test whether the order exists, or it belongs other users.
	_, reply := ss.clientHub.Get(orderKey)
	if !reply.Flag {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(OrderNotFoundMsg)
		return
	}

	// Test whether the order have been paid.
	hasPaid, price, _, _ := parseOrderValue(reply.Value)
	if hasPaid {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write(OrderPaidMsg)
		return
	}

	// ss.coordClients.AsyncPayOrderTxn(orderIDStr, token, price)
	// writer.WriteHeader(http.StatusNoContent)
	// return

	_, txnID := ss.coordClients.AsyncPayOrderTxn(orderIDStr, token, price)
	errCode := ss.coordClients.SyncTxn(txnID)
	flag := normalizeErrCode(errCode)
	// fmt.Println("payOrder", flag)
	switch flag {
	case TxnOK:
		{
			writer.WriteHeader(http.StatusOK)
			writer.Write([]byte("{\"order_id\": \"" + token + "\"}"))
		}
	case TxnOrderPaid:
		{
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(OrderPaidMsg)
		}
	case TxnBalanceInsufficient:
		{
			writer.WriteHeader(http.StatusForbidden)
			writer.Write(BalanceInsufficientMsg)
		}
	}

	return
}

func (ss *ShopServer) queryOneOrder(writer http.ResponseWriter, req *http.Request) {

	var token string
	exist, token := ss.authorize(writer, req, ss.clientHub, false)
	if !exist {
		return
	}

	var reply kv.Reply

	if _, reply = ss.clientHub.Get(OrderKeyPrefix + token); !reply.Flag {
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte("[]"))
		return
	}
	hasPaid, price, _, detail := parseOrderValue(reply.Value)

	var orders [1]Order
	order := &orders[0]
	itemNum := len(detail) // it cannot be zero.
	order.HasPaid = hasPaid
	order.IDStr = token
	order.Items = make([]ItemCount, itemNum)
	order.Total = price
	cnt := 0
	for itemID, itemCnt := range detail {
		if itemCnt != 0 {
			order.Items[cnt].ItemID = itemID
			order.Items[cnt].Count = itemCnt
			cnt++
		}
	}

	body, _ := json.Marshal(orders)
	writer.WriteHeader(http.StatusOK)
	writer.Write(body)
	return
}

// func (ss *ShopServer) queryAllOrders(writer http.ResponseWriter, req *http.Request) {

// 	exist, _ := ss.authorize(writer, req, ss.clientHub, true)
// 	if !exist {
// 		return
// 	}

// 	start := time.Now()

// 	orders := make([]OrderDetail, 0, ss.MaxUserID)

// 	for userID := 1; userID < ss.MaxUserID; userID++ {
// 		userToken := userID2Token(userID)
// 		_, reply := ss.clientHub.Get(OrderKeyPrefix + userToken)
// 		if !reply.Flag {
// 			continue
// 		}
// 		orderInfo := reply.Value
// 		hasPaid, cartIDStr, total := parseOrderInfo(orderInfo)

// 		_, cartDetailKey := getCartKeys(cartIDStr, userToken)
// 		_, reply = ss.clientHub.Get(cartDetailKey)
// 		itemIDAndCounts := parseCartDetail(reply.Value)

// 		itemNum := len(itemIDAndCounts) // it cannot be zero.
// 		orderDetail := OrderDetail{UserID: userID, Order: Order{IDStr: userToken, Items: make([]ItemCount, 0, itemNum), Total: total, HasPaid: hasPaid}}

// 		for itemID, itemCnt := range itemIDAndCounts {
// 			if itemCnt != 0 {
// 				orderDetail.Items = append(orderDetail.Items, ItemCount{ItemID: itemID, Count: itemCnt})
// 			}
// 		}
// 		orders = append(orders, orderDetail)
// 	}
// 	body, _ := json.Marshal(orders)
// 	writer.WriteHeader(http.StatusOK)
// 	writer.Write(body)
// 	end := time.Now().Sub(start)
// 	fmt.Println("queryAllOrders time: ", end.String())
// 	return
// }

// Every action will do authorization except logining.
// @return the flag that indicate whether is authroized or not
func (ss *ShopServer) authorize(writer http.ResponseWriter, req *http.Request, hub *ShardsClientHub, isRoot bool) (bool, string) {
	valid := true
	var authUserID int
	var authUserIDStr string
	req.ParseForm()
	token := req.Form.Get("access_token")
	if token == "" {
		token = req.Header.Get("Access-Token")
	}

	if token == "" {
		valid = false
	} else {
		authUserID = token2UserID(token)
		authUserIDStr = strconv.Itoa(authUserID)

		if isRoot && authUserIDStr != ss.rootToken || !isRoot && (authUserID < 1 || authUserID > ss.MaxUserID) {
			valid = false
		} else {
			if _, reply := hub.Get(TokenKeyPrefix + authUserIDStr); !reply.Flag {
				valid = false
			}
		}
	}

	if !valid {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(InvalidAccessTokenMsg)
		return false, ""
	}
	return true, authUserIDStr
}

func (ss *ShopServer) checkCartExist(cartIDStr, cartKey string, writer http.ResponseWriter, req *http.Request) (flag bool, cartValue string) {
	flag = false
	cartID, _ := strconv.Atoi(cartIDStr)
	if cartID < 1 {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CartNotFoundMsg)
		return
	}
	// Test whether the cart exists,
	var maxCartID = 0
	_, reply := ss.clientHub.Get(CartIDMaxKey)
	if reply.Flag {
		maxCartID, _ = strconv.Atoi(reply.Value)
		if cartID > maxCartID || cartID < 1 {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write(CartNotFoundMsg)
			return
		}
	} else {
		writer.WriteHeader(http.StatusNotFound)
		writer.Write(CartNotFoundMsg)
		return
	}
	// Test whether the cart belongs other users.
	_, reply = ss.clientHub.Get(cartKey)
	if !reply.Flag {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write(NotAuthorizedCartMsg)
		return
	}
	flag = true
	cartValue = reply.Value
	return
}

func isBodyEmpty(writer http.ResponseWriter, req *http.Request) (bool, []byte) {
	const ParseBuffInitLen = 128
	var parseBuff [ParseBuffInitLen]byte
	var ptr, totalReadN = 0, 0
	ret := make([]byte, 0, ParseBuffInitLen/2)

	for readN, _ := req.Body.Read(parseBuff[ptr:]); readN != 0; readN, _ = req.Body.Read(parseBuff[ptr:]) {
		totalReadN += readN
		nextPtr := ptr + readN
		ret = append(ret, parseBuff[ptr:nextPtr]...)
		if nextPtr >= ParseBuffInitLen {
			ptr = 0
		} else {
			ptr = nextPtr
		}
	}

	if totalReadN == 0 {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write(EmptyRequestMsg)
		return true, nil
	}
	return false, ret
}

func normalizeErrCode(errCode int) int {
	// fmt.Println("normalizeErrCode", errCode)
	if errCode <= 0 {
		return errCode
	}
	cnt := 0
	for errCode&0x01 != 0x01 {
		errCode = errCode >> 1
		cnt++
	}
	return 1 << uint(cnt)
}
