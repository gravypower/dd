package helper

import (
	"context"
	"time"

	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
)

// LoopMessages loops over messages, fetching every few seconds and emitting to the channel.
// It terminates if and when the context is stopped.
func LoopMessages(ctx context.Context, conn *dd.Conn, ch chan<- ddapi.DoorStatus) error {
	ticker := time.NewTicker(10 * time.Second) // Send interval
	defer ticker.Stop()

	for {
		// Fetch messages
		messages, err := conn.Messages()
		if err != nil {
			return err
		}

		// Process each message
		for _, m := range messages {
			var out ddapi.DoorStatus
			if decodeErr := m.Decode(&out); decodeErr == nil {
				ch <- out
			}
		}

		// Wait for the next iteration or context cancellation
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Proceed to fetch new messages
		}
	}
}
