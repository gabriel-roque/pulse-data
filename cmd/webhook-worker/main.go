package main

import (
	"github.com/pulse-data/pulse/internal/app"
	"log"
)

func main() {
	if err := app.RunWorker("webhook-worker"); err != nil {
		log.Fatal(err)
	}
}
