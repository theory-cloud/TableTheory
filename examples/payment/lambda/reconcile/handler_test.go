package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
)

func TestHandleRequestStripsLineBreaksFromFailureLog(t *testing.T) {
	// "%\r\n" is an invalid percent-escape, so the record takes the
	// URL-unescape error path, where the raw key reaches the log through the
	// returned error as well as through the key itself.
	const rawKey = "reconciliation/merchant-1/%\r\nINJECTED.csv"

	handler := &ReconcileHandler{}
	event := events.S3Event{Records: []events.S3EventRecord{{
		S3: events.S3Entity{
			Bucket: events.S3Bucket{Name: "reconciliation"},
			Object: events.S3Object{Key: rawKey},
		},
	}}}

	output := captureStdout(t, func() {
		require.NoError(t, handler.HandleRequest(context.Background(), event))
	})

	// Println contributes exactly one trailing newline; any other line break
	// would be an injection from the raw key or from the URL-unescape error.
	require.Equal(t, 1, strings.Count(output, "\n"))
	require.NotContains(t, output, "\r")
	require.NotContains(t, output, rawKey)
	require.True(t, strings.HasPrefix(output, "Error processing file reconciliation/merchant-1/"), output)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer

	defer func() {
		os.Stdout = original
	}()

	fn()

	require.NoError(t, writer.Close())
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	return string(output)
}
