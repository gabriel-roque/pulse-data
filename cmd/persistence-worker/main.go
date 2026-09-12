package main

import (
	"log"

	"github.com/pulse-data/pulse/internal/app"
)

func main() {
	if err := app.RunWorker("persistence-worker"); err != nil {
		log.Fatal(err)
	}
}
