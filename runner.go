package dd

// import (
// 	"encoding/json"
// 	"errors"
// 	"fmt"
// 	"log"
// 	"time"
// )

// var (
// 	// ErrTimeout is returned by Runner when a request response timed out.
// 	ErrTimeout = errors.New("timeout waiting for reply")
// )

// const (
// 	debug              = false
// 	requestTimeout     = time.Second * 10
// 	maximumBackoff     = time.Second * 30
// 	requestSecondCount = 5
// 	updateSecondCount  = 3
// )

// type commandRequest struct {
// 	path     string
// 	input    interface{}
// 	output   interface{}
// 	timeout  time.Duration
// 	response bool
// 	done     chan<- error

// 	id string
// }

// // Runner is a threadsafe runner and request handler.
// type Runner struct {
// 	version        string
// 	commandCh      chan *commandRequest // requests made by users
// 	internalZeroCh chan []byte          // updates of zero data to cache
// 	C              <-chan []byte        // zero data to end user
// }

// // Version returns the software version being emulated.
// func (dcr *Runner) Version() string {
// 	return dcr.version
// }

// func (dcr *Runner) run(dc *Conn, cred Credential) error {
// 	defer func() {
// 		dcr.internalZeroCh <- nil
// 	}()

// 	err := dc.Connect(cred)
// 	if err != nil {
// 		return err
// 	}

// 	backoff := 1
// 	ticker := time.NewTimer(time.Second)
// 	pending := make(map[string]*commandRequest)
// 	failureCh := make(chan string)

// 	for {
// 		select {
// 		case <-ticker.C:
// 			messages, err := dc.Messages(true)
// 			if err != nil {
// 				return err
// 			}
// 			for _, message := range messages {
// 				if message.ProcessID == "" {
// 					dcr.internalZeroCh <- message.DecodedMessage
// 					backoff = -updateSecondCount // check more often in case something is happening
// 					continue
// 				}

// 				if message.ProcessState != nil && *message.ProcessState > 0 {
// 					// ProcessState is +ve if this is a random text message
// 					log.Printf("dropping pid=%s ps=%d: %s", message.ProcessID, *message.ProcessState, string(message.DecodedMessage))
// 					continue
// 				}

// 				prev := pending[message.ProcessID]
// 				if prev == nil {
// 					ps := "<nil>"
// 					if message.ProcessState != nil {
// 						ps = fmt.Sprintf("%d", *message.ProcessState)
// 					}
// 					log.Printf("got unknown message pid=%s ps=%s: %s", message.ProcessID, ps, string(message.DecodedMessage))
// 					continue
// 				}

// 				prev.done <- json.Unmarshal(message.DecodedMessage, prev.output)
// 				delete(pending, message.ProcessID)
// 			}

// 			// every time nothing happens, increase backoff
// 			backoff++
// 			duration := time.Second
// 			if backoff <= 1 {
// 				// nothing
// 			} else {
// 				duration *= time.Duration(backoff * backoff)
// 				if duration > maximumBackoff {
// 					duration = maximumBackoff
// 				}
// 			}
// 			ticker.Reset(duration)

// 		case id := <-failureCh:
// 			prev := pending[id]
// 			if prev != nil {
// 				log.Printf("request `%v` failed due to timeout", id)
// 				delete(pending, id)
// 				prev.done <- ErrTimeout
// 			}

// 		case req := <-dcr.commandCh:
// 			req.id, err = dc.Request(req.path, req.input)
// 			if err != nil {
// 				if req.done != nil {
// 					req.done <- err // this individual request failed
// 				}
// 				return err // but also restart our runner
// 			}

// 			if !req.response {
// 				req.done <- nil
// 				break
// 			}

// 			// store to be resolved later
// 			pending[req.id] = req
// 			go func() {
// 				time.Sleep(req.timeout)
// 				failureCh <- req.id
// 			}()

// 			// reset backoff
// 			backoff = -requestSecondCount
// 			if !ticker.Stop() {
// 				<-ticker.C
// 			}
// 			ticker.Reset(0)
// 		}
// 	}
// }

// // NewRunner builds a new thread-safe runner for this API.
// func NewRunner(dc Conn, cred Credential) *Runner {
// 	zeroCh := make(chan []byte)
// 	runner := &Runner{
// 		version:        dc.Version,
// 		commandCh:      make(chan *commandRequest),
// 		internalZeroCh: make(chan []byte),
// 		C:              zeroCh,
// 	}

// 	// status update task
// 	go func() {
// 		for {
// 			pending := <-runner.internalZeroCh
// 			var rest [][]byte

// 			select {
// 			case more := <-runner.internalZeroCh:
// 				if more == nil {
// 					continue // error, restart
// 				}
// 				rest = append(rest, more)

// 			case zeroCh <- pending:
// 				// great (sent to user), pop if there's more waiting
// 				if len(rest) == 0 {
// 					continue
// 				}
// 				pending = rest[0]
// 				rest = rest[1:]
// 			}
// 		}
// 	}()

// 	// primary runner task
// 	go func() {
// 		for {
// 			err := runner.run(&dc, cred)
// 			log.Printf("got runner err: %+v", err)
// 			time.Sleep(requestTimeout) // just delay for a bit
// 			// TODO: shutdown behavior
// 		}
// 	}()

// 	return runner
// }

// // Request makes an immediate synchronous request with no expected result.
// func (dcr *Runner) Request(path string, input interface{}) error {
// 	ch := make(chan error)
// 	dcr.commandCh <- &commandRequest{
// 		path:  path,
// 		input: input,
// 		done:  ch,
// 	}
// 	return <-ch
// }

// // Fetch makes a request to a path, waiting for its output or a timeout.
// func (dcr *Runner) Fetch(path string, input interface{}, output interface{}) error {
// 	ch := make(chan error)
// 	dcr.commandCh <- &commandRequest{
// 		path:     path,
// 		input:    input,
// 		output:   output,
// 		timeout:  requestTimeout,
// 		response: true,
// 		done:     ch,
// 	}
// 	return <-ch
// }
