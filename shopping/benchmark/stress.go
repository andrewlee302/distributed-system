package main

import (
	"bytes"
	"distributed-system/util"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"time"
)

//----------------------------------
// Stress Settings
//----------------------------------
const (
	MAX_COCURRENCY = 1024 * 1024
	TIMEOUT_SEC    = 10
)

//----------------------------------
// Stress Abstracts
//----------------------------------
type Worker struct {
	r   *Reporter
	ctx struct {
		addrs []string
	}
}

type Context struct {
	c       *http.Client
	w       *Worker
	user    User
	orderId string
	cartId  string
}

type TimeInterval struct {
	start    int64
	end      int64
	interval int64
}

type Reporter struct {
	payMade         chan bool
	payIntervals    chan TimeInterval
	requestSent     chan bool
	userCurr        chan User
	numOrders       int
	cocurrency      int
	nOrderOk        int
	nOrderErr       int
	nOrderTotal     int
	nOrderPerSec    []int
	payCosts        []time.Duration
	nRequestOk      int
	nRequestErr     int
	nRequestTotal   int
	nRequestPerSec  []int
	timeStampPerSec []int
	startAt         time.Time
	elapsed         time.Duration
}

//----------------------------------
// Entity Abstracts
//----------------------------------
type User struct {
	Id          int
	Username    string
	Password    string
	AccessToken string
}

type Item struct {
	Id    int `json:"id"`
	Price int `json:"price"`
	Stock int `json:"stock"`
}

//----------------------------------
// Request JSON Bindings
//----------------------------------
type RequestLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RequestCartAddItem struct {
	ItemId int `json:"item_id"`
	Count  int `json:"count"`
}

type RequestMakeOrder struct {
	CartId string `json:"cart_id"`
}

type RequestPayOrder struct {
	OrderId string `json:"order_id"`
}

//----------------------------------
// Response JSON Bindings
//----------------------------------
type ResponseLogin struct {
	UserId      int    `json:"user_id"`
	Username    string `json:"username"`
	AccessToken string `json:"access_token"`
}

type ResponseGetItems []Item

type ResponseCreateCart struct {
	CartId string `json:"cart_id"`
}

type ResponseMakeOrder struct {
	OrderId string `json:"order_id"`
}

type ResponsePayOrder struct {
	OrderId string `json:"order_id"`
}

type ResponseQueryOrder struct {
	Id   string `json:"id"`
	Item []struct {
		ItemId int `json:"item_id"`
		Count  int `json:"count"`
	} `json:"items"`
	Total int `json:"total"`
}

//----------------------------------
// Global Variables
//----------------------------------
var (
	users           = make([]User, 0)    // users
	items           = make(map[int]Item) // map[item.Id]item
	isDebugMode     = false
	isReportToRedis = false
)

//----------------------------------
// Data Initialization
//----------------------------------
// Load all data.
func LoadData(userCsv, itemCsv string) {
	fmt.Printf("Load users from user file..")
	LoadUsers(userCsv)
	fmt.Printf("OK\n")
	fmt.Printf("Load items from item file..")
	LoadItems(itemCsv)
	fmt.Printf("OK\n")
}

// Load users from user csv file
func LoadUsers(userCsv string) {
	// read users
	if file, err := os.Open(userCsv); err == nil {
		reader := csv.NewReader(file)
		defer file.Close()
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			userId, _ := strconv.Atoi(strs[0])
			if userId != 0 {
				userName := strs[1]
				password := strs[2]
				users = append(users, User{Id: userId, Username: userName, Password: password})
			}
		}
	} else {
		panic(err.Error())
	}

	// root user
	// ss.rootToken = userId2Token(ss.UserMap["root"].Id)
}

// Load items from item csv file
func LoadItems(itemCsv string) {
	// read items
	if file, err := os.Open(itemCsv); err == nil {
		reader := csv.NewReader(file)
		defer file.Close()
		for strs, err := reader.Read(); err == nil; strs, err = reader.Read() {
			itemId, _ := strconv.Atoi(strs[0])
			price, _ := strconv.Atoi(strs[1])
			stock, _ := strconv.Atoi(strs[2])
			items[itemId] = Item{Id: itemId, Price: price, Stock: stock}
		}
	} else {
		panic(err.Error())
	}
}

//----------------------------------
// Request Utils
//----------------------------------
// Build url with path and parameters.
func (w *Worker) Url(path string, params url.Values) string {
	// random choice one host for load balance
	i := rand.Intn(len(w.ctx.addrs))
	addr := w.ctx.addrs[i]
	s := fmt.Sprintf("http://%s%s", addr, path)
	if params == nil {
		return s
	}
	p := params.Encode()
	return fmt.Sprintf("%s?%s", s, p)
}

// Get json from uri.
func (w *Worker) Get(c *http.Client, url string, bind interface{}) (int, error) {
	r, err := c.Get(url)
	if err != nil {
		if r != nil {
			ioutil.ReadAll(r.Body)
			r.Body.Close()
		}
		return 0, err
	}
	defer r.Body.Close()
	err = json.NewDecoder(r.Body).Decode(bind)
	if bind == nil {
		return r.StatusCode, nil
	}
	return r.StatusCode, err
}

// Post json to uri and get json response.
func (w *Worker) Post(c *http.Client, url string, data interface{}, bind interface{}) (int, error) {
	var body io.Reader
	if data != nil {
		bs, err := json.Marshal(data)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(bs)
	}
	r, err := c.Post(url, "application/json", body)
	if err != nil {
		if r != nil {
			ioutil.ReadAll(r.Body)
			r.Body.Close()
		}
		return 0, err
	}
	defer r.Body.Close()
	err = json.NewDecoder(r.Body).Decode(bind)
	if bind == nil {
		return r.StatusCode, nil
	}
	return r.StatusCode, err
}

// Patch url with json.
func (w *Worker) Patch(c *http.Client, url string, data interface{}, bind interface{}) (int, error) {
	bs, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(bs))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		if res != nil {
			ioutil.ReadAll(res.Body)
			res.Body.Close()
		}
		return 0, err
	}
	defer res.Body.Close()
	err = json.NewDecoder(res.Body).Decode(bind)
	if res.StatusCode == http.StatusNoContent || bind == nil {
		return res.StatusCode, nil
	}
	return res.StatusCode, err
}

//----------------------------------
//  Order Handle Utils
//----------------------------------
// Random choice a item. Dont TUCAO this function,
// it works and best O(1).
func GetRandItem() Item {
	for {
		idx := rand.Intn(len(items))
		item, ok := items[idx+1]
		if ok {
			return item
		}
	}
}

//----------------------------------
//  Work Job Context
//----------------------------------
func (ctx *Context) UrlWithToken(path string) string {
	user := ctx.user
	params := url.Values{}
	params.Add("access_token", user.AccessToken)
	return ctx.w.Url(path, params)
}

func (ctx *Context) Login() bool {
	user := ctx.user
	data := &RequestLogin{user.Username, user.Password}
	body := &ResponseLogin{}
	url := ctx.w.Url("/login", nil)
	statusCode, err := ctx.w.Post(ctx.c, url, data, body)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request login error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusOK {
		ctx.user.AccessToken = body.AccessToken
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false
}

func (ctx *Context) GetItems() bool {
	// body := &ResponseGetItems{}
	url := ctx.UrlWithToken("/items")
	statusCode, err := ctx.w.Get(ctx.c, url, nil)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request get items error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusOK {
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false
}

func (ctx *Context) CreateCart() bool {
	body := &ResponseCreateCart{}
	url := ctx.UrlWithToken("/carts")
	statusCode, err := ctx.w.Post(ctx.c, url, nil, body)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request create carts error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusOK {
		ctx.cartId = body.CartId
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false
}

func (ctx *Context) CartAddItem() bool {
	path := fmt.Sprintf("/carts/%s", ctx.cartId)
	url := ctx.UrlWithToken(path)
	item := GetRandItem()
	data := &RequestCartAddItem{item.Id, 1}
	statusCode, err := ctx.w.Patch(ctx.c, url, data, nil)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request error cart add item error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusNoContent {
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false
}

func (ctx *Context) MakeOrder() bool {
	if !ctx.Login() || !ctx.GetItems() || !ctx.CreateCart() {
		return false
	}
	// count := rand.Intn(3) + 1
	count := 2
	for i := 0; i < count; i++ {
		if !ctx.CartAddItem() {
			return false
		}
	}
	data := &RequestMakeOrder{ctx.cartId}
	body := &ResponseMakeOrder{}
	url := ctx.UrlWithToken("/orders")
	statusCode, err := ctx.w.Post(ctx.c, url, data, body)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request make order error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusOK {
		ctx.orderId = body.OrderId
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false
}

func (ctx *Context) PayOrder() bool {
	if !ctx.MakeOrder() {
		return false
	}
	url := ctx.UrlWithToken("/pay")
	data := &RequestPayOrder{ctx.orderId}
	body := &ResponsePayOrder{}
	statusCode, err := ctx.w.Post(ctx.c, url, data, body)
	if err != nil {
		if isDebugMode {
			fmt.Printf("Request pay order error: %v\n", err)
		}
		ctx.w.r.requestSent <- false
		return false
	}
	if statusCode == http.StatusOK {
		ctx.w.r.requestSent <- true
		return true
	}
	ctx.w.r.requestSent <- false
	return false

}

//----------------------------------
// Worker
//----------------------------------
func NewWorker(addrs []string, r *Reporter) *Worker {
	w := &Worker{}
	w.r = r
	w.ctx.addrs = addrs
	return w
}
func (w *Worker) Work() {
	ctx := &Context{}
	ctx.w = w
	t := &http.Transport{}
	ctx.c = &http.Client{
		Timeout:   TIMEOUT_SEC * time.Second,
		Transport: t,
	}
	for {
		// t.CloseIdleConnections()
		startAt := time.Now()
		ctx.user = <-w.r.userCurr
		w.r.payMade <- ctx.PayOrder()
		endAt := time.Now()
		w.r.payIntervals <- TimeInterval{start: startAt.UnixNano(), end: endAt.UnixNano(), interval: endAt.Sub(startAt).Nanoseconds()}
	}
}

//----------------------------------
// Statstics Reporter
//----------------------------------
// Create reporter
func NewReporter(numOrders int, cocurrency int) *Reporter {
	return &Reporter{
		make(chan bool, cocurrency),
		make(chan TimeInterval, cocurrency),
		make(chan bool, cocurrency),
		make(chan User, cocurrency),
		numOrders,
		cocurrency,
		0,
		0,
		0,
		make([]int, 0),
		make([]time.Duration, 0),
		0,
		0,
		0,
		make([]int, 0),
		make([]int, 0),
		time.Now(),
		0,
	}
}

// Start reporter
func (r *Reporter) Start() {
	r.startAt = time.Now()
	go func() {
		t := time.NewTicker(1 * time.Second)
		for {
			nOrderOk := r.nOrderOk
			nRequestOk := r.nRequestOk
			<-t.C
			nOrderPerSec := r.nOrderOk - nOrderOk
			r.nOrderPerSec = append(r.nOrderPerSec, nOrderPerSec)
			nRequestPerSec := r.nRequestOk - nRequestOk
			r.nRequestPerSec = append(r.nRequestPerSec, nRequestPerSec)
			r.timeStampPerSec = append(r.timeStampPerSec, time.Now().Second())
			fmt.Printf("Finished orders: %d\n", nOrderPerSec)
		}
	}()
	go func() {
		for {
			payMade := <-r.payMade
			payInterval := <-r.payIntervals
			if payMade {
				r.nOrderOk = r.nOrderOk + 1
				r.payCosts = append(r.payCosts, time.Duration(payInterval.interval))
			} else {
				r.nOrderErr = r.nOrderErr + 1
			}
			r.nOrderTotal = r.nOrderTotal + 1
			if r.nOrderTotal >= r.numOrders {
				r.Stop()
			}
		}
	}()
	go func() {
		for {
			requestSent := <-r.requestSent
			if requestSent {
				r.nRequestOk = r.nRequestOk + 1
			} else {
				r.nRequestErr = r.nRequestErr + 1
			}
			r.nRequestTotal = r.nRequestTotal + 1
		}
	}()
	for i := 0; i < len(users); i++ {
		r.userCurr <- users[i]
	}
	timeout := time.After(TIMEOUT_SEC * time.Second)
	for r.nOrderTotal < r.numOrders {
		select {
		case <-timeout:
			r.Stop()
		}
	}
	r.Stop()
}

// Stop the reporter and exit full process.
func (r *Reporter) Stop() {
	r.elapsed = time.Since(r.startAt)
	r.Report()
	os.Exit(0)
}

// Report stats to console and redis.
func (r *Reporter) Report() {
	//---------------------------------------------------
	// Report to console
	//---------------------------------------------------
	sort.Ints(r.nOrderPerSec)
	sort.Ints(r.nRequestPerSec)
	nOrderPerSecMax := MeanOfMaxFive(r.nOrderPerSec)
	nOrderPerSecMin := MeanOfMinFive(r.nOrderPerSec)
	nOrderPerSecMean := Mean(r.nOrderPerSec)
	nRequestPerSecMax := MeanOfMaxFive(r.nRequestPerSec)
	nRequestPerSecMin := MeanOfMinFive(r.nRequestPerSec)
	nRequestPerSecMean := Mean(r.nRequestPerSec)
	sort.Ints(r.nRequestPerSec)
	payCostNanoseconds := []float64{}
	for i := 0; i < len(r.payCosts); i++ {
		payCostNanoseconds = append(payCostNanoseconds, float64(r.payCosts[i].Nanoseconds()))
	}
	sort.Float64s(payCostNanoseconds)
	msTakenTotal := int(r.elapsed.Nanoseconds() / 1000000.0)
	msPerOrder := MeanFloat64(payCostNanoseconds) / 1000000.0
	msPerRequest := SumFloat64(payCostNanoseconds) / 1000000.0 / float64(r.nRequestOk)
	//---------------------------------------------------
	// Report to console
	//---------------------------------------------------
	fmt.Print("\nStats\n")
	fmt.Printf("Concurrency level:         %d\n", r.cocurrency)
	fmt.Printf("Time taken for tests:      %dms\n", msTakenTotal)
	fmt.Printf("Complete requests:         %d\n", r.nRequestOk)
	fmt.Printf("Failed requests:           %d\n", r.nRequestErr)
	fmt.Printf("Complete orders:           %d\n", r.nOrderOk)
	fmt.Printf("Failed orders:             %d\n", r.nOrderErr)
	fmt.Printf("Time per request:          %.2fms\n", msPerRequest)
	fmt.Printf("Time per order:            %.2fms\n", msPerOrder)
	fmt.Printf("Request per second:        %d (max)  %d (min)  %d(mean)\n", nRequestPerSecMax, nRequestPerSecMin, nRequestPerSecMean)
	fmt.Printf("Order per second:          %d (max)  %d (min)  %d (mean)\n\n", nOrderPerSecMax, nOrderPerSecMin, nOrderPerSecMean)
	fmt.Printf("Percentage of orders made within a certain time (ms)\n")
	if len(payCostNanoseconds) == 0 {
		return
	}
	percentages := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 95, 95.5, 96, 96.5, 97, 97.5, 98, 98.5, 99, 99.9, 99.99, 100}
	for _, percentage := range percentages {
		idx := int(percentage * float64(len(payCostNanoseconds)) / float64(100.0))
		if idx > 0 {
			idx = idx - 1
		} else {
			idx = 0
		}
		payCostNanosecond := payCostNanoseconds[idx]
		fmt.Printf("%.2f%%\t%d ms\n", percentage, int(payCostNanosecond/1000000.0))
	}
}

//----------------------------------
// Math util functions
//----------------------------------
func MeanOfMaxFive(sortedArr []int) int {
	if len(sortedArr) == 0 {
		return 0
	}
	if len(sortedArr) == 1 {
		return sortedArr[0]
	}
	if len(sortedArr) == 2 {
		return sortedArr[1]
	}
	sortedArr = sortedArr[1 : len(sortedArr)-1]
	if len(sortedArr) > 5 {
		return Mean(sortedArr[len(sortedArr)-5:])
	}
	return sortedArr[len(sortedArr)-1]
}

func MeanOfMinFive(sortedArr []int) int {
	if len(sortedArr) == 0 {
		return 0
	}
	if len(sortedArr) == 1 {
		return sortedArr[0]
	}
	if len(sortedArr) == 2 {
		return sortedArr[0]
	}
	sortedArr = sortedArr[1 : len(sortedArr)-1]
	if len(sortedArr) > 5 {
		return Mean(sortedArr[0:5])
	}
	return sortedArr[0]
}

func Mean(arr []int) int {
	if len(arr) == 0 {
		return 0
	}
	sum := 0
	for i := 0; i < len(arr); i++ {
		sum = sum + arr[i]
	}
	return int(float64(sum) / float64(len(arr)))
}

func MeanFloat64(arr []float64) float64 {
	return SumFloat64(arr) / float64(len(arr))
}

func SumFloat64(arr []float64) float64 {
	if len(arr) == 0 {
		return 0
	}
	sum := 0.0
	for i := 0; i < len(arr); i++ {
		sum = sum + arr[i]
	}
	return sum
}

//----------------------------------
// Main
//----------------------------------
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	//----------------------------------
	// Arguments parsing and validation
	//----------------------------------
	config := flag.String("f", "cfg.json", "config file")
	cocurrency := flag.Int("c", 1000, "request cocurrency")
	numOrders := flag.Int("n", 1000, "number of orders to perform")
	debug := flag.Bool("d", false, "debug mode")
	// reportRedis := flag.Bool("r", true, "report to local redis")
	flag.Parse()
	cfg := util.ParseCfg(*config)
	// if flag.NFlag() == 0 {
	// 	flag.PrintDefaults()
	// 	os.Exit(1)
	// }
	if *debug {
		isDebugMode = true
	}
	// if *reportRedis {
	// 	isReportToRedis = true
	// }
	//----------------------------------
	// Validate cocurrency
	//----------------------------------
	if *cocurrency > MAX_COCURRENCY {
		fmt.Printf("Exceed max cocurrency (is %d)", MAX_COCURRENCY)
		os.Exit(1)
	}
	//----------------------------------
	// Load users/items and work
	//----------------------------------
	LoadData(cfg.UserCSV, cfg.ItemCSV)
	reporter := NewReporter(*numOrders, *cocurrency)
	for i := 0; i < *cocurrency; i++ {
		go func() {
			w := NewWorker(cfg.APPAddrs, reporter)
			w.Work()
		}()
	}
	// start reporter
	reporter.Start()
}
