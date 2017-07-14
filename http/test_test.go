package http

import "testing"
import "runtime"
import "os"
import "time"
import "fmt"

func TestBasic(t *testing.T) {
}

func TestConcurrence(t *testing.T) {
	runtime.GOMAXPROCS(4)
}
