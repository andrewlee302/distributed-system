package main

import (
	"distributed-system/shopping"
	"distributed-system/twopc"
	"distributed-system/util"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/pprof"
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

	var resolveErr error
	blocked := false
	if *parti {
		for _, pptAddr := range cfg.KVStoreAddrs {
			if err := resolveAddr(host_ips, pptAddr); err == nil {
				blocked = true
				go shopping.NewShoppingTxnKVStoreService(cfg.Protocol, pptAddr, cfg.CoordinatorAddr)
			} else {
				resolveErr = err
			}
		}
	}

	if *coord {
		if err := resolveAddr(host_ips, cfg.CoordinatorAddr); err == nil {
			blocked = true
			go shopping.NewShoppingTxnCoordinator(cfg.Protocol, cfg.CoordinatorAddr,
				cfg.KVStoreAddrs, keyHashFunc, cfg.TimeoutMS)
		} else {
			resolveErr = err
		}
	}

	if *web {
		for _, appAddr := range cfg.APPAddrs {
			if err := resolveAddr(host_ips, appAddr); err == nil {
				blocked = true
				go shopping.InitService(cfg.Protocol, appAddr, cfg.CoordinatorAddr, cfg.UserCSV, cfg.ItemCSV,
					cfg.KVStoreAddrs, keyHashFunc)
			} else {
				resolveErr = err
			}
		}
	}

	if blocked {
		block := make(chan bool)
		<-block
	} else if resolveErr != nil {
		fmt.Println(resolveErr)
		flag.PrintDefaults()
	} else {
		fmt.Println("You should indicate a specific service: -s -c -p.")
		flag.PrintDefaults()
	}
}

func resolveAddr(host_ips map[string]struct{}, addr string) error {
	if domain, _, err := net.SplitHostPort(addr); err == nil {
		if ips, err := net.LookupHost(domain); err == nil {
			if _, ok := host_ips[ips[0]]; ok {
				return nil
			} else {
				return errors.New(fmt.Sprintf("%s doesn't apply in local network\n", ips[0]))
			}
		} else {
			return err
		}
	} else {
		return err
	}
}
