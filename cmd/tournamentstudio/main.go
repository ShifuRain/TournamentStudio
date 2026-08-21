package main

import (
	"log"
	"net/http"
	"os"

	"tournamentstudio/internal/server"
	"tournamentstudio/internal/store"
)

func main() {
	dbPath := os.Getenv("TOURNAMENTSTUDIO_DB")
	if dbPath == "" {
		dbPath = "tournamentstudio.db"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.DB.Close()

	s := server.New(st)
	addr := ":8080"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, s); err != nil {
		log.Fatal(err)
	}
}
