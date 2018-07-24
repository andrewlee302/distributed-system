# Distributed System Course

The fantastic experiment for education on distributed system, including
the ubiquitous communication over the Web, classic techniques for scaling and
efficient storage, a popular application case, aims to incredibly capture the 
essences of the difficult but useful distributed system theory, such 
as the two-phase protocol and the paxos consensus protocol. It covers the
common technique issues almost in all the distributed systems, including 
communication, data consistency, parallism, concurrence, replication. The 
techniques take attentions on the performance, fault-tolerance, scaling and
user-friendliness, which are important metrics for distributed systems.

[![Go Report Card](https://goreportcard.com/badge/github.com/andrewlee302/distributed-system)](https://goreportcard.com/report/github.com/andrewlee302/distributed-system)
[![MIT license](http://img.shields.io/badge/license-MIT-brightgreen.svg)](http://opensource.org/licenses/MIT)
[![GoDoc](https://godoc.org/github.com/andrewlee302/distributed-system?status.svg)](https://godoc.org/github.com/andrewlee302/distributed-system)

## Modules or library

1. HTTP library over TCP and Unix domain socket
2. 2PC library.
3. Tiny key-value storage module.
4. Transcation-supported shopping web service module over the sharded 
key-value storage.
5. Paxos library.
6. Key-value storage module over paxos.
7. Utility for the resouce pool.
7. A final system, including web service, sharded and replicated key-value 
storage.

## Testing
Every package has its unit test for functions and performance. (TODO) The final 
system will be tested in a container-style way.