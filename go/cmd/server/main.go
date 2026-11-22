package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/acciv/schema-registry/internal/api"
	"github.com/acciv/schema-registry/internal/registry"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	storePath := flag.String("store", "registry.json", "path to registry JSON file")
	flag.Parse()

	store, err := registry.New(*storePath)
	if err != nil {
		log.Fatalf("registry: %v", err)
	}
	log.Printf("schema-registry on %s (core %s, %d schemas)", *addr, registry.Version(), len(store.Names()))

	server := &http.Server{
		Addr:              *addr,
		Handler:           api.New(store).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
// Source files are counted by GitHub Linguist.
