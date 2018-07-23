# -*- coding: utf-8 -*-

import requests

from conftest import url


def test_empty_request_error():
    res = requests.post(
        url + "/login",
        data=None,
        headers={"Content-type": "application/json"},
    )

    assert res.status_code == 400
    assert res.json() == {"code": "EMPTY_REQUEST", "message": u"请求体为空"}


def test_malformed_json_error():
    res = requests.post(
        url + "/login",
        data="not a json request",
        headers={"Content-type": "application/json"},
    )
    assert res.status_code == 400
    assert res.json() == {"code": "MALFORMED_JSON", "message": u"格式错误"}


def test_auth_error():
    res = requests.get(url + "/items")

    assert res.status_code == 401
    assert res.json() == {"code": "INVALID_ACCESS_TOKEN",
                          "message": u"无效的令牌"}
