package util

import (
	"encoding/json"
	"fmt"
	"os"
)

type Cfg struct {
	Protocol        string
	APPAddrs        []string
	CoordinatorAddr string
	KVStoreAddrs    []string
	ItemCSV         string
	UserCSV         string
	TimeoutMS       int64
}

func ParseCfg(f string) *Cfg {
	file, _ := os.Open(f)
	defer file.Close()
	decoder := json.NewDecoder(file)
	cfg := &Cfg{}
	err := decoder.Decode(&cfg)
	if err != nil {
		fmt.Println("error:", err)
	}
	return cfg
}
