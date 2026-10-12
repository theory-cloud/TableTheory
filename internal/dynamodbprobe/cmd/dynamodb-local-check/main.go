// Command dynamodb-local-check exits 0 when the endpoint speaks the DynamoDB
// protocol and non-zero with a clear message otherwise.
//
// The example Makefiles call it from their docker-up targets so that a
// non-DynamoDB service listening on the local port is not mistaken for
// DynamoDB Local.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/theory-cloud/tabletheory/v4/internal/dynamodbprobe"
)

const checkTimeout = 3 * time.Second

func main() {
	endpoint := flag.String("endpoint", dynamodbprobe.DefaultEndpoint, "DynamoDB endpoint to check")
	flag.Parse()

	os.Exit(run(*endpoint))
}

func run(endpoint string) int {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	if err := dynamodbprobe.CheckEndpoint(ctx, endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "DynamoDB Local not detected: %v\n", err)
		return 1
	}

	fmt.Printf("DynamoDB Local detected at %s\n", endpoint)
	return 0
}
