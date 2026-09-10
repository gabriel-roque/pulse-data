package main

import (
	"github.com/pulse-data/pulse/internal/app"
	"log"
)

func main() {
	if err := app.RunHTTP("ingestion"); err != nil {
		log.Fatal(err)
	}
}
