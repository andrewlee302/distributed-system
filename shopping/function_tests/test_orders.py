# -*- coding: utf-8 -*-

from __future__ import absolute_import

from conftest import (
    json_get, token_gen, item_gen,
    item_store, new_cart, make_order)


def test_get_orders():
    _, token = next(token_gen)
    # new user should have no orders
    res = json_get("/orders", token)
    assert res.status_code == 200
    assert len(res.json()) == 0


def test_make_order():

    uid, token = next(token_gen)
    cart_id = new_cart(token)
    item_items = [next(item_gen)]

    # make order success
    res = make_order(uid, token, cart_id, item_items)
    assert res.status_code == 200
    order_id = res.json()["order_id"]
    assert len(order_id) > 0

    # verify query return the same order
    orders = json_get("/orders", token).json()
    assert len(orders) == 1
    q_order = orders[0]
    assert q_order["id"] == order_id
    assert q_order["items"] == item_items

    # verify order info correct
    _price = lambda i: item_store[i]["price"]
    assert q_order["total"] == sum(
        _price(item["item_id"]) * item["count"] for item in q_order["items"])

    # test only one order can be made
    res2 = make_order(uid, token, cart_id, item_items)
    assert res2.status_code == 403
    assert res2.json() == {"code": "ORDER_OUT_OF_LIMIT",
                           "message": u"每个用户只能下一单"}


def test_make_order_cart_not_owned_error():
    uid, token1 = next(token_gen)
    cart_id1 = new_cart(token1)
    _, token2 = next(token_gen)

    res = make_order(uid, token2, cart_id1, [next(item_gen)])
    assert res.status_code == 401
    assert res.json() == {"code": "NOT_AUTHORIZED_TO_ACCESS_CART",
                          "message": u"无权限访问指定的篮子"}
