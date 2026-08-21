package main

import (
	"log"
	"net/http"

	"tournamentstudio/internal/server"
)

func main() {
	s := server.New()
	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}
