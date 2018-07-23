package main

import (
	"distributed-system/twopc"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/pprof"
	"rush-shopping/shopping"
	"rush-shopping/util"
	"strings"
)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	web := flag.Bool("s", false, "shopping web service")
	coord := flag.Bool("c", false, "shopping kvstore coordinator")
	parti := flag.Bool("p", false, "shopping kvstore participant")
	config := flag.String("f", "cfg.json", "config file")

	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	cfg := util.ParseCfg(*config)

	keyHashFunc := twopc.DefaultKeyHashFunc

	addrs, _ := net.InterfaceAddrs()
	host_ips := make(map[string]struct{})
	for _, addr := range addrs {
		host_ips[strings.Split(addr.String(), "/")[0]] = struct{}{}
	}

	var err error
	blocked := false
	if *parti {
		for _, pptAddr := range cfg.KVStoreAddrs {
			if domain, _, err := net.SplitHostPort(pptAddr); err == nil {
				if ips, err := net.LookupHost(domain); err == nil {
					if _, ok := host_ips[ips[0]]; ok {
						blocked = true
						go shopping.NewShoppingTxnKVStoreService(cfg.Protocol, pptAddr, cfg.CoordinatorAddr)
					} else {
						fmt.Println("here")
					}
				} else {
					fmt.Println(err)
				}
			} else {
				fmt.Println(err)
			}
		}
	}

	if *coord {
		if domain, _, err := net.SplitHostPort(cfg.CoordinatorAddr); err == nil {
			if ips, err := net.LookupHost(domain); err == nil {
				if _, ok := host_ips[ips[0]]; ok {
					blocked = true
					go shopping.NewShoppingTxnCoordinator(cfg.Protocol, cfg.CoordinatorAddr,
						cfg.KVStoreAddrs, keyHashFunc, cfg.TimeoutMS)
				} else {
					fmt.Printf("%s doesn't apply in local network\n", ips[0])
				}
			} else {
				fmt.Println(err)
			}
		} else {
			fmt.Println(err)
		}
	}

	if *web {
		for _, appAddr := range cfg.APPAddrs {
			if domain, _, err := net.SplitHostPort(appAddr); err == nil {
				if ips, err := net.LookupHost(domain); err == nil {
					if _, ok := host_ips[ips[0]]; ok {
						blocked = true
						go shopping.InitService(cfg.Protocol, appAddr, cfg.CoordinatorAddr, cfg.UserCSV, cfg.ItemCSV,
							cfg.KVStoreAddrs, keyHashFunc)
					}
				}
			}
		}
	}
	if blocked {
		block := make(chan bool)
		<-block
	} else {
		if err != nil {
			fmt.Println(err)
		}
		flag.PrintDefaults()
	}
}
