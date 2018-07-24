#!/usr/bin/env sh
#
# Function tests
#
# You must compile the start_service binary and then start the following 
# services before tests:
# 1. Particpant service: ./start_service -p
# 2. Coordinator service: ./start_service -c
# 3. Shopping web service: ./start_service -s
#
# Except for the coordinator service, the particpant services and web services
# could be more than one single service, which is all decided by cfg.json.
#
# The function test config below is isolated from the start_service, but 
# actually it should keep the configs consistent.

# Test config.
export APP_HOST="localhost"
export APP_PORT="10001"
export ITEM_CSV="data/items.csv"
export USER_CSV="data/users.csv"

# Run tests.
pytest function_tests/test_errors.py function_tests/test_login.py \
function_tests/test_items.py function_tests/test_carts.py function_tests/test_orders.py \
function_tests/test_pay.py function_tests/test_stock.py
