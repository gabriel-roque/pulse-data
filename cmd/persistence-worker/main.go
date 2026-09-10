package main

import (
	"github.com/pulse-data/pulse/internal/app"
	"log"
	"os"
)

func main() {
	_ = os.Setenv("PULSE_SERVICE", "persistence-worker")
	if err := app.RunWorker("persistence-worker"); err != nil {
		log.Fatal(err)
	}
}
