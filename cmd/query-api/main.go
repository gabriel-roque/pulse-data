package main

import (
	"github.com/pulse-data/pulse/internal/app"
	"log"
)

func main() {
	if err := app.RunHTTP("query-api"); err != nil {
		log.Fatal(err)
	}
}
