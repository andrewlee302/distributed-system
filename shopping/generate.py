#!/usr/bin/env python

import random
import os
import sys


USER_CSV = "data/users.csv"
ITEM_CSV = "data/items.csv"
INIT_ROOT_MONEY = 0
INIT_MONEY = 1000
ZERO_MONEY = 0
NUM_USERS = 5000
NUM_ITEMS = 20000
MAX_STOCK = 200
MAX_PRICE = 300

if not os.path.exists("data"):
    os.mkdir("data")
elif not os.path.isdir("data"):
    print "It's not a directory, ./data."
    exit(1)

# Id for normal users starts from 1.
print "Generate %d users -> %s" %(NUM_USERS, USER_CSV)
with open(USER_CSV, 'w') as f:
    # root
    f.write("%d,%s,%s,%d\n" % (0, "root", "root", INIT_ROOT_MONEY))
    for i in range(1, NUM_USERS):
        if random.random() > 0.05:
            name = "andrew%d" %i
            balance = INIT_MONEY
        else:
            name = "zero_andrew%d" %i
            balance = ZERO_MONEY
        password = name
        f.write("%d,%s,%s,%d\n" % (i, name, password, balance))

# Id for items starts from 1.
print "Generate %d items -> %s" %(NUM_ITEMS, ITEM_CSV)
with open(ITEM_CSV, 'w') as f:
    for i in range(1, NUM_ITEMS+1):
        price = random.randint(1,MAX_PRICE+1)
        stock = random.randint(1,MAX_STOCK)
        f.write("%d,%d,%d\n" % (i, price, stock))
