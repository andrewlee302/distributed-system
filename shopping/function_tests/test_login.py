# -*- coding: utf-8 -*-
import random

import requests

from conftest import (
    json_get,
    token_gen,
    url,
    user_store,
)


def test_login_success():
    uid = random.choice(list(user_store.keys()))
    username, password, stock = user_store[uid]

    data = {"username": username, "password": password}
    res = requests.post(
        url + "/login",
        json=data,
        headers={"Content-type": "application/json"},
    )

    assert res.status_code == 200
    assert res.json()["user_id"] == uid
    assert res.json()["username"] == username
    assert len(res.json().get("access_token", "")) > 0


def test_login_error():
    uid = random.choice(list(user_store.keys()))
    username, password, stock = user_store[uid]

    data = {"username": username, "password": password + password[:1]}
    res = requests.post(
        url + "/login",
        json=data,
        headers={"Content-type": "application/json"})
    assert res.status_code == 403
    assert res.json() == {"code": "USER_AUTH_FAIL",
                          "message": u"用户名或密码错误"}

    data = {"username": username + username[:1], "password": password}

    res = requests.post(url + "/login",
        json=data,
        headers={"Content-type": "application/json"})
    assert res.status_code == 403
    assert res.json() == {"code": "USER_AUTH_FAIL",
                          "message": u"用户名或密码错误"}


def test_login_post_data():
    uid = random.choice(list(user_store.keys()))
    username, password, stock = user_store[uid]

    data = {"username": username, "password": password}
    res = requests.post(
        url + "/login",
        data=data,
        headers={"Content-type": "application/json"})

    assert res.status_code == 400
    assert res.json() == {"code": "MALFORMED_JSON", "message": u"格式错误"}


#def test_token_not_too_simple():
#
#    def _valid(tk):
#        return json_get("/items", tk).status_code == 200
#
#    uids, usernames, passwords, tokens = [], [], [], []
#    for i in range(5):
#        uid, token = next(token_gen)
#        username, password, stock = user_store[uid]
#
#        # simply don't use them directly, it's way too simple
#        assert uid != token
#        assert username != token
#        assert password != token
#
#        uids.append(uid)
#        usernames.append(username)
#        passwords.append(password)
#        tokens.append(token)
#
#    # username & password should never be contained in token
#    # it may occasionally occur in token since it's random string, so
#    # test 5 random user to avoid mistake
#    assert not all(u in t for u, t in zip(usernames, tokens))
#    assert not all(p in t for p, t in zip(passwords, tokens))
#
#    # uid may occur in token since it's only numbers, but don't make it too
#    # easily guessed
#    for uid, tk in zip(uids, tokens):
#        if str(uid) in token:
#            for i in range(1, 10):
#                assert not _valid(token.replace(str(uid), str(int(uid) + i)))
#                assert not _valid(token.replace(str(uid), str(int(uid) - i)))
