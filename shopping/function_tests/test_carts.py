# -*- coding: utf-8 -*-

from __future__ import absolute_import

from conftest import json_post, json_patch, token_gen, item_gen, new_cart


def test_new_cart():
    _, token = next(token_gen)

    res = json_post("/carts", token)
    assert res.status_code == 200
    cart_id = res.json().get("cart_id", "")
    assert len(cart_id) > 0

    # carts should not equal to each other
    cart_id2 = json_post("/carts", token).json().get("cart_id")
    assert cart_id != cart_id2


def test_add_item():
    _, token = next(token_gen)
    cart_id = new_cart(token)

    # success add 3 item
    for i in range(3):
        res = json_patch("/carts/%s" % cart_id, token, next(item_gen))
        assert res.status_code == 204
        assert len(res.content) == 0

    # fail if try to add more
    res = json_patch("/carts/%s" % cart_id, token, next(item_gen))
    assert res.status_code == 403
    assert res.json() == {"code": "ITEM_OUT_OF_LIMIT",
                          "message": u"篮子中物品数量超过了三个"}


def test_del_item():
    _, token = next(token_gen)
    cart_id = new_cart(token)
    item = next(item_gen)
    json_patch("/carts/%s" % cart_id, token, item)

    # delete existing item
    item["count"] = -1
    res = json_patch("/carts/%s" % cart_id, token, item)
    assert res.status_code == 204
    assert len(res.content) == 0

    # delete non-exist item
    res = json_patch("/carts/%s" % cart_id, token, next(item_gen))
    assert res.status_code == 204
    assert len(res.content) == 0


def test_cart_not_found_error():
    _, token = next(token_gen)

    res = json_patch("/carts/-1", token, next(item_gen))
    assert res.status_code == 404
    assert res.json() == {"code": "CART_NOT_FOUND", "message": u"篮子不存在"}


def test_item_not_found_error():
    _, token = next(token_gen)
    cart_id = new_cart(token)

    item = {"item_id": -1, "count": 2}
    res = json_patch("/carts/%s" % cart_id, token, item)

    assert res.status_code == 404
    assert res.json() == {"code": "ITEM_NOT_FOUND", "message": u"物品不存在"}


def test_cart_not_owned_error():
    _, token1 = next(token_gen)
    _, token2 = next(token_gen)
    cart_id2 = new_cart(token2)

    res = json_patch("/carts/%s" % cart_id2, token1, next(item_gen))
    assert res.status_code == 401
    assert res.json() == {"code": "NOT_AUTHORIZED_TO_ACCESS_CART",
                          "message": u"无权限访问指定的篮子"}
