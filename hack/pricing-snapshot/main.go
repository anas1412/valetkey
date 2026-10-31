// Command pricing-snapshot refreshes internal/pricing/snapshot.json.gz, the price
// list built into the binary for clusters that can't fetch prices themselves.
//
//	go run ./hack/pricing-snapshot
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/anas1412/valetkey/internal/pricing"
)

func main() {
	cat, err := pricing.Fetcher{}.Fetch(context.Background(), time.Now())
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range cat.Sources {
		if !s.OK {
			log.Fatalf("%s failed: %s", s.Name, s.Error)
		}
	}
	data, err := pricing.Encode(cat)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("internal/pricing/snapshot.json.gz", data, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d providers, %d bytes", len(cat.Providers), len(data))
}
