package fewviolations

import (
	"fmt"
	"log"
)

// ProcessData does some work and has exactly 3 violations:
// 1. fmt.Println in a non-main package
// 2. panic outside main/init
// 3. import "log" in a non-main package

func ProcessData(data string) error {
	fmt.Println("processing:", data)
	if data == "" {
		panic("data must not be empty")
	}
	log.Println("done")
	return nil
}
