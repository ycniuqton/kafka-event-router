// Command routerctl runs a route config over a JSONL file of events without Kafka
// (milestone 1): `routerctl run --config route.json --events in.jsonl [--trace] [--now T]`.
package main

import (
	"context"
	"os"
)

func main() {
	os.Exit(int(NewCLI(os.Stdout, os.Stderr).Main(context.Background(), os.Args[1:])))
}
