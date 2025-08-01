package main

import (
	"fmt"
	"log"

	"mcglab.ru/bsdmp/v3/bsdmp"
)

func main() {
	pack := bsdmp.NewPack(bsdmp.CompressionNone, 0, nil, nil, nil)
	err := pack.Title([]struct {
		Name     string
		TypeCode bsdmp.FieldType
		TypeLen  bsdmp.FieldSize
	}{
		{"name", bsdmp.FieldTypeString, bsdmp.FieldSizeB255},
		{"age", bsdmp.FieldTypeInt, bsdmp.FieldSizeG4},
		{"active", bsdmp.FieldTypeBool, bsdmp.FieldSizeB255},
	})
	if err != nil {
		log.Fatal(err)
	}
	pack.Frame(map[string]interface{}{"name": "Alice", "age": int64(25)})
	pack.Frame(map[string]interface{}{"name": "Bob", "age": int64(30), "active": true})
	data, err := pack.Pack()
	if err != nil {
		log.Fatal(err)
	}

	unpack, err := bsdmp.NewUnpack(data, true)
	if err != nil {
		log.Fatal(err)
	}
	for _, frame := range unpack.Frames {
		fmt.Println(frame)
	}
}
