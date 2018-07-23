# Key Value format

## The courrent version
```
  token:<userIDStr> - "1"
  balance:<token> - <balance>
  cartID - "1000"
  cart:<cartIDStr>:<token> - 2.<itemIDStr1>:<itemCnt1>;<itemIDStr2>:<itemCnt2>
  order:<token> - <flag>|<totalPrice>|2.<itemIDStr1>:<itemCnt1>;<itemIDStr2>:<itemCnt2>
  items_size - "200" // The number of types of items.
  n_users_size - "200"  // The number of the normal users.
  items_stock:<itemIDStr> - "100" // The stock of some item.
  items_price:<itemIDStr> - "100" // The price of some item.
```

## The obsolete version
```
  token:<userIDStr> - "1"
  balance:<token> - <balance>
  cartID - "1000"
  cart_n:<cartIDStr>:<token> - "2"
  cart_d:<cartIDStr>:<token> - <itemIDStr1>:<itemCnt1>;<itemIDStr2>:<itemCnt2>
  order:<token> - <flag>:<cartIDStr>:<totalPrice>
  items_size - "200" // The number of types of items.
  n_users_size - "200"  // The number of the normal users.
  items_stock:<itemIDStr> - "100" // The stock of some item.
  items_price:<itemIDStr> - "100" // The price of some item.
```