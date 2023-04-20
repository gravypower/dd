package helper

import (
	"log"
	"time"

	"github.com/samthor/dd"
	ddapi "github.com/samthor/dd/api"
)

// LoopMessages loops over messages, fetching every few seconds and logging to screen.
// It does not do anything more useful right now.
func LoopMessages(conn *dd.Conn) {
	for {
		messages, err := conn.Messages()
		if err != nil {
			log.Fatalf("Err fetching background messages: %v", err)
		}
		for _, m := range messages {
			var out ddapi.DoorStatus
			m.Decode(&out)
			log.Printf("Got status message: %+v", out)
		}
		time.Sleep(time.Second * 4)
	}
}
