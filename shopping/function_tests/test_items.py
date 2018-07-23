# -*- coding: utf-8 -*-

from conftest import item_store, json_get, token_gen


def test_items():

    def _req():
        _, token = next(token_gen)
        return json_get("/items", token)

    res = _req()
    assert res.status_code == 200

    items = res.json()
    assert len(items) == len(item_store)

    items2 = _req().json()
    assert items == items2

    for item in items:
        assert item["id"] in item_store
        assert isinstance(item["stock"], int) and item["stock"] <= 1000
        assert isinstance(item["price"], int) and item["price"] > 0
        assert item_store[item["id"]]["price"] == item["price"]
