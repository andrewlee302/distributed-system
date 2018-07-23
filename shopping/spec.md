# 接口规范

需要实现基于 HTTP/JSON 的 RESTful API，至少包含以下接口:

1. <a href="#login">登录</a>
1. <a href="#items">查询库存</a>
1. <a href="#carts">创建篮子</a>
1. <a href="#item">添加物品</a>
1. <a href="#order">下单</a>
2. <a href="#pay">支付</a>
1. <a href="#orders">查询订单</a>
1. <a href="#admin-orders">后台接口－查询订单</a>

除登录接口外，其他接口需要传入登录接口得到的access_token，access_token无效或者为空会直接返回401异常：

```
401 Unauthorized
{
    "code": "INVALID_ACCESS_TOKEN",
    "message": "无效的令牌"
}
```

其中后台接口只有用root用户登录后的access_token才能访问；非后台接口root用户无法访问。

其中access_token需要支持：parameter和http header两种认证方式。客户端提供其中一个即可通过认证。

```
# by parameter
GET /items?access_token=xxx

# by http header
GET /items Access-Token:xxx
```


如果需要传参的接口，传过来的body为空。则返回400异常:

```
400 Bad Request
{
    "code": "EMPTY_REQUEST",
    "message": "请求体为空"
}
```

如果需要传参的接口，传过来的请求体json格式有误。则返回400异常:

```
400 Bad Request
{
    "code": "MALFORMED_JSON",
    "message": "格式错误"
}
```

<a name="login" />

## 登录

`POST /login`

##### 请求体

参数名 | 类型 | 描述
---|---|---
username | string | 用户名
password | string | 密码

#### 请求示例

```
POST /login
{
    "username": "robot",
    "password": "robot"
}
```

#### 响应示例

```
200 OK
{
    "user_id": 1,
    "username": "robot",
    "access_token": "xxx"
}
```

#### 异常示例

用户名不存在或者密码错误：

```
403 Forbidden
{
    "code": "USER_AUTH_FAIL",
    "message": "用户名或密码错误"
}
```

<a name="items" />
## 查询库存

`GET /items`

#### 请求示例

```
GET /items?access_token=xxx
```

#### 响应示例

```
200 OK
[
    {"id": 1, "price": 12, "stock": 99},
    {"id": 2, "price": 10, "stock": 89},
    {"id": 3, "price": 22, "stock": 91}
]
```

<a name="carts" />
## 创建篮子

`POST /carts`

#### 请求示例

```
POST /carts?access_token=xxx

```

#### 响应示例

```
200 OK
{
    "cart_id ": "e0c68eb96bd8495dbb8fcd8e86fc48a3"
}
```

<a name="item" />
## 添加物品

`PATCH /carts/:cart_id`

##### 请求体

参数名 | 类型 | 描述
---|---|---
item_id | int | 添加的物品id
count | int | 添加的物品数量

#### 请求示例

```
PATCH /carts/e0c68eb96bd8495dbb8fcd8e86fc48a3?access_token=xxx
{
    "item_id": 2,
    "count": 1
}
```

#### 响应示例

```
204 No content
```

#### 异常示例

篮子不存在：

```
404 Not Found
{
    "code": "CART_NOT_FOUND",
    "message": "篮子不存在"
}
```

篮子不属于当前用户：

```
401 Unauthorized
{
    "code": "NOT_AUTHORIZED_TO_ACCESS_CART",
    "message": "无权限访问指定的篮子"
}
```

物品数量超过篮子最大限制：

```
403 Forbidden
{
    "code": "ITEM_OUT_OF_LIMIT",
    "message": "篮子中物品数量超过了三个"
}
```

物品不存在：

```
404 Not Found
{
    "code": "ITEM_NOT_FOUND",
    "message": "物品不存在"
}
```

<a name="order" />
## 下单

`POST /orders`
下单成功后，购物车将不会释放，用户可以继续修改。

##### 请求体

参数名 | 类型 | 描述
---|---|---
cart_id | string | 篮子id

#### 请求示例

```
POST /orders?access_token=xxx
{
    "cart_id": "e0c68eb96bd8495dbb8fcd8e86fc48a3"
}
```

#### 响应示例

```
200 OK
{
    "order_id ": "someorderid"
}
```

#### 异常示例
篮子不属于当前用户

```
401 Unauthorized
{
    "code": "NOT_AUTHORIZED_TO_ACCESS_CART",
    "message": "无权限访问指定的篮子"
}
```

篮子不存在

```
404 Not Found
{
    "code": "CART_NOT_FOUND",
    "message": "篮子不存在"
}
```

购物车为空

```
403 Forbidden
{
    "code": "CART_EMPTY",
    "message": "购物车为空"
}
```

物品库存不足

```
403 Forbidden
{
    "code": "ITEM_OUT_OF_STOCK",
    "message": "物品库存不足"
}
```

超过下单次数限制

```
403 Forbidden
{
    "code": "ORDER_OUT_OF_LIMIT",
    "message": "每个用户只能下一单"
}
```
<a name="pay" />
## 支付

`POST /pay`

##### 请求体

参数名 | 类型 | 描述
---|---|---
order_id | string | 订单id

#### 请求示例

```
POST /pay?access_token=xxx
{
    "order_id": "e0c68eb96bd8495dbb8fcd8e86fc48a3"
}
```

#### 响应示例

```
200 OK
{
    "order_id ": "someorderid"
}
```

#### 异常示例

订单不属于当前用户

```
401 Unauthorized
{
    "code": "NOT_AUTHORIZED_TO_ACCESS_ORDER",
    "message": "无权限访问指定的订单"
}
```

订单不存在

```
404 Not Found
{
    "code": "ORDER_NOT_FOUND",
    "message": "订单不存在"
}
```


订单已支付

```
403 Forbidden
{
    "code": "ORDER_PAID",
    "message": "订单已支付"
}
```

余额不足

```
403 Forbidden
{
    "code": "BALANCE_INSUFFICIENT",
    "message": "余额不足"
}
```

<a name="orders" />
## 查询订单
`GET /orders`
不应该包含个数为0的商品。

#### 请求示例

```
GET /orders?access_token=xxx
```

#### 响应示例

```
200 OK
[
    {
        "id": "someorderid",
        "items": [
            {"item_id": 2, "count": 1}
        ],
        "total": 10
        "paid": true
    }
]
```


<a name="admin-orders" />
## 后台接口－查询订单
`GET /admin/orders`
不应该包含个数为0的商品。

#### 请求示例

```
GET /admin/orders?access_token=xxx
```

#### 响应示例

```
200 OK
[
    {
        "id": "someorderid",
        "user_id": "1",
        "items": [
            {"item_id": 2, "count": 1}
        ],
        "total": 10
    }
]
```

