package main

import (
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	"github.com/theory-cloud/tabletheory/v6/examples/blog/handlers"
)

func main() {
	handler, err := handlers.NewCommentHandler()
	if err != nil {
		log.Fatalf("initialize comments handler: %v", err)
	}
	lambda.Start(handler.HandleRequest)
}
