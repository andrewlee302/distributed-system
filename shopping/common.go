package shopping

// The type of ID for Item and User is int. To differ the two from others, we
// specify the field IDStr to indicate the its type is string, otherwise int.

import (
	"bytes"
	"strconv"
	"strings"
)

type Item struct {
	ID    int `json:"id"`
	Price int `json:"price"`
	Stock int `json:"stock"`
}

type LoginJson struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Order struct {
	// if total < 0, then is a order
	IDStr   string      `json:"id"`
	Items   []ItemCount `json:"items"`
	Total   int         `json:"total"`
	HasPaid bool        `json:"paid"`
}

type ItemCount struct {
	ItemID int `json:"item_id"`
	Count  int `json:"count"`
}

// OrderDetail is for GET /admin/orders
type OrderDetail struct {
	Order
	UserID int `json:"user_id"`
}

type CartIDJson struct {
	IDStr string `json:"cart_id"`
}

type OrderIDJson struct {
	IDStr string `json:"order_id"`
}

type UserIDAndPass struct {
	ID       int
	Password string
}

func getCartKey(cartIDStr, token string) (cartKey string) {
	cartKey = "cart:" + cartIDStr + ":" + token
	return
}

func userID2Token(userID int) string {
	return strconv.Itoa(userID)
}

func token2UserID(token string) int {
	if id, err := strconv.Atoi(token); err == nil {
		return id
	} else {
		panic(err)
	}
}

func composeOrderValue(hasPaid bool, price, num int, detail map[int]int) string {
	var info [3]string
	if hasPaid {
		info[0] = OrderPaidFlag
	} else {
		info[0] = OrderUnpaidFlag
	}
	info[1] = strconv.Itoa(price)
	info[2] = composeCartValue(num, detail)
	return strings.Join(info[:], "|")
}

func parseOrderValue(value string) (hasPaid bool, price, num int, detail map[int]int) {
	info := strings.Split(value, "|")
	if info[0] == OrderPaidFlag {
		hasPaid = true
	} else {
		hasPaid = false
	}

	if len(info) != 3 {
		panic(value)
	}
	price, _ = strconv.Atoi(info[1])
	num, detail = parseCartValue(info[2])
	return
}

// cartValue shouldn't be "". Otherwise return 0, blank map.
// 2.1:3;2:4
// 0
func parseCartValue(cartValue string) (num int, detail map[int]int) {
	detail = make(map[int]int)
	if cartValue == "" {
		return 0, detail
	}
	vs := strings.Split(cartValue, ".")
	num, _ = strconv.Atoi(vs[0])
	cnt := 0
	if len(vs) == 2 {
		itemStrs := strings.Split(vs[1], ";")
		for _, itemStr := range itemStrs {
			info := strings.Split(itemStr, ":")
			if len(info) != 2 {
				panic("cartValue format error")
			}
			itemID, _ := strconv.Atoi(info[0])
			itemCnt, _ := strconv.Atoi(info[1])
			cnt += itemCnt
			detail[itemID] = itemCnt
		}
	} else if len(vs) > 2 {
		panic("cartValue format error")
	}

	if num != cnt {
		panic("total value error")
	}
	return
}

func composeCartValue(num int, cartDetail map[int]int) (cartDetailStr string) {
	cnt := 0
	var buffer bytes.Buffer
	buffer.WriteString(strconv.Itoa(num) + ".")
	for itemID, itemCnt := range cartDetail {
		cnt += itemCnt
		buffer.WriteString(strconv.Itoa(itemID))
		buffer.WriteString(":")
		buffer.WriteString(strconv.Itoa(itemCnt))
		buffer.WriteString(";")
	}
	buffer.Truncate(buffer.Len() - 1)
	if num != cnt {
		panic("total value error")
	}
	return buffer.String()
}
