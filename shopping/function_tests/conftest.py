# -*- coding: utf-8 -*-

from __future__ import absolute_import

import collections
import json
import logging
import os
import random
import csv

import pymysql
import pymysql.cursors
import requests

logging.basicConfig(level=logging.WARNING)

def _conf():
    conf = {}
    app_host = os.environ['APP_HOST']
    app_port = os.environ['APP_PORT']
    if app_host == "":
        conf['APP_HOST'] = "localhost"
    else:
        conf['APP_HOST'] = app_host
    if app_port == "":
        conf['APP_PORT'] = "8080"
    else:
        conf['APP_PORT'] = app_port
    conf['URL'] = "http://" + conf['APP_HOST'] + ":" + conf['APP_PORT']
    conf['ITEM_CSV'] = os.environ['ITEM_CSV']
    conf['USER_CSV'] = os.environ['USER_CSV']
    return conf

def _token(username, password):
    data = {"username": username, "password": password}
    res = requests.post(
        url + "/login",
        data=json.dumps(data),
        headers={"Content-type": "application/json"})
    assert res.status_code == 200
    return res.json()["access_token"]

# basic conf
conf = _conf()
url = conf["URL"]

# admin info
admin_username = "root"
admin_password = "root"
admin_token = _token(admin_username, admin_password)

def _load_users():
    with open(conf["USER_CSV"]) as f:
        reader = csv.reader(f)
        return {int(row[0]): (row[1], row[2], row[3]) for row in reader if row[0] != "0"}

def _load_items():
    with open(conf["ITEM_CSV"]) as f:
        reader = csv.reader(f)
        return {int(row[0]): {"price": int(row[1]), "stock":int(row[2])} for row in reader}

# test data
user_store = _load_users()
item_store = _load_items()
order_store = {}

# req utils
_session = requests.session()
_session.headers.update({"Content-type": "application/json"})
_session.mount("http://", requests.adapters.HTTPAdapter(max_retries=10))

def json_get(path, tk):
    return _session.get(url + path, headers={"Access-Token": tk}, timeout=3)

def json_post(path, tk, data=None):
    return _session.post(
        url + path,
        json=data,
        headers={"Access-Token": tk},
        timeout=3)

def json_patch(path, tk, data):
    return _session.patch(
        url + path,
        json=data,
        headers={"Access-Token": tk},
        timeout=3)

def _token_gen():
    uids = list(user_store.keys())
    random.shuffle(uids)
    for uid in uids:
        username, password, _ = user_store[uid]
        yield uid, _token(username, password)

def _balance_insufficient_token_gen():
    uids = list(user_store.keys())
    random.shuffle(uids)
    for uid in uids:
        username, password, _ = user_store[uid]
        if len(username) > 4 and username[:4] == "zero":
            yield uid, _token(username, password)

def _balance_ok_token_gen():
    uids = list(user_store.keys())
    random.shuffle(uids)
    for uid in uids:
        username, password, _ = user_store[uid]
        if len(username) <= 4 or username[:4] != "zero":
            yield uid, _token(username, password)

def _item_gen():
    item_ids = list(item_store.keys())
    while True:
        item_id = random.choice(item_ids)
        yield {"item_id": item_id, "count": 1}

token_gen = _token_gen()
balance_insufficient_token_gen = _balance_insufficient_token_gen()
balance_ok_token_gen = _balance_ok_token_gen()
item_gen = _item_gen()

# utils
def new_cart(token):
    res = json_post("/carts", token)
    return res.status_code == 200 and res.json()["cart_id"]

def make_order(uid, token, cart_id, item_items):
    for item_item in item_items:
        json_patch("/carts/%s" % cart_id, token, item_item)
    res = json_post("/orders", token, {"cart_id": cart_id})

    # decrease item stock, and record success order
    if res.status_code == 200:
        counts = collections.Counter()
        for item_item in item_items:
            counts[item_item["item_id"]] += item_item["count"]

        for item_id, count in counts.items():
            item_store[item_id]["stock"] -= count
        order_store[res.json()["order_id"]] = {"user_id": uid, "items": item_items}
    return res

def pay_order(uid, token, order_id):
    res = json_post("/pay", token, {"order_id": order_id})
    return res

def simple_make_order(item_items):
    uid, token = next(token_gen)
    return make_order(uid, token, new_cart(token), item_items)
