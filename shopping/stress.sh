#!/usr/bin/env sh
#
# Stress tests
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
# The stress test config is decides by cfg.json.

# Compile.
go build benchmark/stress.go

# Run stress test.
if [[ $? == 0 ]] then; ./stress -d -c 100 -d 10000;fi
