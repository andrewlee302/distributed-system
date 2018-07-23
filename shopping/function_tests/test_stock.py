# -*- coding: utf-8 -*-

import random

import concurrent.futures
from concurrent.futures.thread import ThreadPoolExecutor

from conftest import (
    admin_token,
    item_store,
    json_get,
    order_store,
    simple_make_order,
    token_gen, new_cart, json_patch, json_post)

def buy_to_stock(item_id, target_stock):
    remain_stock = max(item_store[item_id]["stock"] - target_stock, 0)
    while remain_stock > 0:
        count = min(remain_stock, 3)
        res = simple_make_order([{"item_id": item_id, "count": count}])

        # should success when item have remain stock
        assert res.status_code == 200

        remain_stock -= count


def test_item_stock_consistency():

    item_id = random.choice(list(item_store.keys()))
    buy_to_stock(item_id, 0)

    # should fail when item out of stock
    res = simple_make_order([{"item_id": item_id, "count": 1}])
    assert res.status_code == 403
    assert res.json() == {"code": "ITEM_OUT_OF_STOCK",
                          "message": u"物品库存不足"}


#def test_admin_query_orders():
#    print "admin_token", admin_token
#    res = json_get("/admin/orders", admin_token)
#    assert res.status_code == 200
#
#    q_orders = res.json()
#    l = sorted([o["id"] for o in q_orders])
#    r = sorted(order_store.keys())
#    # print l
#    # print r
#    if len(l) > len(r):
#        for cnt in range(len(l)):
#            if cnt == len(l)-1 or l[cnt] != r[cnt]:
#                print l[cnt]
#                print q_orders[cnt]
#                break
#
#    assert len(q_orders) == len(order_store)
#
#    for q_order in q_orders:
#        order_id = q_order["id"]
#        assert order_id in order_store
#        assert q_order["user_id"] == order_store[order_id]["user_id"]
#        if q_order["items"] != order_store[order_id]["items"]:
#            print order_id
#        assert q_order["items"] == order_store[order_id]["items"]

def test_item_not_oversold_under_concurrent():

    TEST_ITEM_COUNT = 5
    TEST_ITEM_STOCK = 10

    # random choose items with more than 10 stock
    test_item_ids = random.sample(
        [f for f, s in item_store.items() if s["stock"] >= TEST_ITEM_STOCK],
        TEST_ITEM_COUNT)
    for item_id in test_item_ids:
        buy_to_stock(item_id, TEST_ITEM_STOCK)
        assert item_store[item_id]["stock"] == TEST_ITEM_STOCK

    # enumerate all item items
    total_item_items = []
    for item_id in test_item_ids:
        remain_stock = item_store[item_id]["stock"]
        items = [{"item_id": item_id, "count": 1}] * remain_stock
        total_item_items.extend(items)
    assert len(total_item_items) == TEST_ITEM_COUNT * TEST_ITEM_STOCK

    # try to buy as much as twice of the stock
    test_item_items = total_item_items * 2
    random.shuffle(test_item_items)

    # prepare carts & tokens, each carts contains 2 items
    cart_ids, tokens, items_list = [], [], []
    for item_items in zip(test_item_items[::2], test_item_items[1::2]):
        _, token = next(token_gen)
        cart_id = new_cart(token)

        for item in item_items:
            res = json_patch("/carts/%s" % cart_id, token, item)
            assert res.status_code == 204

        cart_ids.append(cart_id)
        tokens.append(token)
        items_list.append(item_items)


    def _make(cart_id, token, item_items):
        res = json_post("/orders", token, {"cart_id": cart_id})
        if res.status_code == 200:
            for item_item in item_items:
                item_store[item_item["item_id"]]["stock"] -= 1
        return res

    # make order with prepared carts, using 3 concurrent threads
    # allow sell slower (remain stock > 0)
    # best sell all and correct (remain stock == 0)
    # disallow oversold (remain stock < 0)
    with ThreadPoolExecutor(max_workers=3) as executor:
        future_results = [
            executor.submit(_make, ct, tk, fs)
            for ct, tk, fs in zip(cart_ids, tokens, items_list)]
        concurrent.futures.wait(future_results, timeout=30)

    # test not oversold
    for item_id in test_item_ids:
        assert item_store[item_id]["stock"] >= 0
