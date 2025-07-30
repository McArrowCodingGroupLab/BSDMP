package main

import (
	"encoding/hex"
	"fmt"

	"mcglab.ru/bsdmp/v2/bsdmp"
)

func main() {
	client := bsdmp.NewClient(1, bsdmp.CompressionGZIP, []byte("FRAME>"), []byte("<FRAME>"))
	client.Format([]string{"name", "age", "active"})
	client.AddFrame([]interface{}{"Alice", 25, true})
	client.AddFrame([]interface{}{"Bob", 30, false})

	encoded, err := client.Encode()
	if err != nil {
		panic(err)
	}

	fmt.Printf("HEX: %x\n", encoded)

	hexStr := "0100000001000000780000001f8b08000000000000ff348c410a023110046bb249fc81fa0c51bc8910416f5efcc118460944058dbe5f24eea569e8ae7240040ea774dc6f27c0a6d729e0b9ebcd6440af2611cdad7c4cfa2ec01c08a45ab239966b4f7bbeedcff7fce967c0c0ee7176ac16818bd6d778fa060000ffff97d8e9e87e000000"
	encoded2, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}

	server := bsdmp.NewServer()
	err = server.Decode(encoded2)
	if err != nil {
		panic(err)
	}

	for i, frame := range server.Frames {
		fmt.Printf("=== Frame %d ===\n", i+1)
		for key, value := range frame {
			fmt.Printf("%s=%v\n", key, value)
		}
		fmt.Println()
	}
}
