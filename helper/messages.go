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
	for {
		messages, err := conn.Messages()
		if err != nil {
			return err
		}
		for _, m := range messages {
			var out ddapi.DoorStatus
			err = m.Decode(&out)
			if err == nil {
				// Try to send all messages in case we got multiple.
				ch <- out
			}
		}
		if err != nil {
			return err
		}

		timer := time.NewTimer(time.Second * 10)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil
		}
	}
}
