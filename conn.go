package dd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	RemoteAPIBase  = "version2.smartdoordevices.com"
	SDKPort        = 8991
	DefaultPort    = 8989
	DefaultVersion = "2.21.1"
)

var (
	ErrTimeout = errors.New("RPC call timeout")
	logger     = logrus.New()
)

func init() {
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})
	logger.SetLevel(logrus.InfoLevel)
}

// Conn is a connection to the service.
type Conn struct {
	Version     string // version number to send
	Host        string // hostname
	RequestMode bool   // whether to "request" changes, used for talking to an online server
	Debug       bool   // whether to log debug

	cred   Credential   // cached creds
	client *http.Client // cached optional client

	processID      string // random process ID to use in requests
	sessionID      string // session ID returned from server
	nextAccess     int    // the next timestamp to use (millis)
	sessionSecret  []byte // to calculate sessionSignature (from server)
	phoneSecret    []byte // to calculate phoneSignature, derived from cred.PhoneSecret
	phoneSecretRaw []byte // raw secret, UTF-8 bytes of string

	sequenceIDSuffix int // incremented suffix (to track replies)
	pendingMessages  []*Message

	genericRequestMutex sync.Mutex
	unresolvedMutex     sync.Mutex
	unresolvedRPC       map[string]chan *Message
}

// Credential holds login/connect credentials.
type Credential struct {
	PhoneSecret   string `json:"phoneSecret,omitempty"` // phone secret
	BaseStation   string `json:"bsid,omitempty"`        // base station ID
	Phone         string `json:"phoneId,omitempty"`     // phone ID
	PhonePassword string `json:"phonePassword,omitempty"`
	UserPassword  string `json:"userPassword,omitempty"`
}

type requestConfig struct {
	data            []byte
	path            string
	requestIfOnline bool // does this need to be "requested" via /app/res/request
}

// genericRequest is what we actually marshal as JSON for any request.
type genericRequest struct {
	requestIfOnline bool // does this need to be "requested" via /app/res/request
	dataPayload

	Credential
	SessionID         string `json:"sessionId,omitempty"`
	ProcessID         string `json:"processId,omitempty"`
	SessionSignature  string `json:"sessionSig,omitempty"`
	PhoneSignature    string `json:"phoneSig,omitempty"`
	Path              string `json:"path,omitempty"`
	CommunicationType int    `json:"communicationType,omitempty"`
}

type genericResponse struct {
	SessionSignature string `json:"sessionSig"`
	RawMessages      string `json:"messages"`
	Message          string `json:"message"`
	BaseStation      string `json:"bsid"`
	inlineResponse   []byte
	dataPayload

	// Fields from a connect response
	SessionID           string `json:"sessionId"`
	IsBasestationOnline bool   `json:"isBasestationOnline"`
	HubVersion          int    `json:"hubVersion"`
	CommunicationType   int    `json:"communicationType"`
	SessionSecret       string `json:"sessionSecret"`
	ServerTime          int    `json:"serverTime"`
	IsAdmin             bool   `json:"isAdmin"`
}

// Messages decodes the list of Message instances in this genericResponse, if any.
func (gr *genericResponse) Messages() (out []*Message, err error) {
	if len(gr.RawMessages) == 0 {
		return nil, nil // nothing in this payload
	}
	err = json.Unmarshal([]byte(gr.RawMessages), &out)
	return out, err
}

// Message is a log event from the device. It's returned as part of genericResponse.
type Message struct {
	AppTimeout     int    `json:"appTimeout"`
	ProcessID      string `json:"processId"`
	Sequence       int    `json:"sequence"`
	ProcessState   *int   `json:"processState"` // nb. sometimes is unset
	PhoneSignature string `json:"phoneSig"`
	Type           int    `json:"type"`
	dataPayload

	DecodedMessage []byte `json:"-"` // actual decoded message
}

func (m *Message) Decode(target interface{}) error {
	err := json.NewDecoder(bytes.NewBuffer(m.DecodedMessage)).Decode(target)
	if err != nil {
		return err
	}

	// Log the decrypted message
	logger.WithFields(logrus.Fields{
		"processID": m.ProcessID,
		"sequence":  m.Sequence,
		"type":      m.Type,
		"message":   string(m.DecodedMessage), // Convert to string for readability
	}).Debug("Decrypted message")
	return nil
}

type connectResponseData struct {
	UserAccess struct {
		IsAccessReady                 bool   `json:"isAccessReady"`
		NextAccess                    int    `json:"nextAccess"`
		IsExpired                     bool   `json:"isExpired"`
		IsCurrentlyRestricted         bool   `json:"isCurrentlyRestricted"`
		DescriptionRestrictionDetails string `json:"descriptionRestrictionDetails"`
		HashCode                      int    `json:"hashCode"`
		NextRestricted                int    `json:"nextRestricted"`
		IsHubClockAccurate            bool   `json:"isHubClockAccurate"`
		DescriptionNextEvent          string `json:"descriptionNextEvent"`
		OneTimeLimit                  int    `json:"oneTimeLimit"`
		HasRestrictions               bool   `json:"hasRestrictions"`
	} `json:"userAccess"`
	IsPasswordExpired bool `json:"isPasswordExpired"`
	IsAdmin           bool `json:"isAdmin"`
}

type SimpleRequestTarget int

const (
	DefaultTarget SimpleRequestTarget = iota
	SDKTarget
	RemoteTarget
)

type SimpleRequest struct {
	Path   string              // Path for request
	Target SimpleRequestTarget // Where to call
	Input  interface{}
	Output interface{}
}

// SimpleRequest performs a simple request to our device, without session logic.
func (dc *Conn) SimpleRequest(arg SimpleRequest) error {
	if len(arg.Path) > 0 && arg.Path[0] != '/' {
		return fmt.Errorf("path must start with /, got: %v", arg.Path)
	}

	jsonBytes, err := json.Marshal(arg.Input)
	if err != nil {
		return err
	}

	var url string
	switch arg.Target {
	case RemoteTarget:
		url = fmt.Sprintf("https://%s%s", RemoteAPIBase, arg.Path)
	case SDKTarget:
		url = fmt.Sprintf("https://%s:%d%s", dc.Host, SDKPort, arg.Path)
	case DefaultTarget:
		url = fmt.Sprintf("https://%s:%d%s", dc.Host, DefaultPort, arg.Path)
	default:
		return fmt.Errorf("unknown target: %v", arg.Target)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}

	logger.WithFields(logrus.Fields{
		"url":     url,
		"payload": string(jsonBytes),
	}).Debug("Sending request")

	version := dc.Version
	if version == "" {
		version = DefaultVersion
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("sddAndroid-%s-LGE Nexus 5X(28)", version))
	req.Header.Set("version", version)
	req.Header.Set("platform", "android")

	// If we haven't yet created an HTTP client, do so now
	if dc.client == nil {
		customTransport := http.DefaultTransport.(*http.Transport).Clone()
		// WARNING: For production, you should NOT use InsecureSkipVerify = true.
		customTransport.TLSClientConfig.InsecureSkipVerify = true
		dc.client = &http.Client{Transport: customTransport}
	}

	resp, err := dc.client.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		if cerr := Body.Close(); cerr != nil {
			logger.WithError(cerr).Error("failed to close response body")
		}
	}(resp.Body)

	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logger.WithFields(logrus.Fields{
		"statusCode": resp.StatusCode,
		"response":   string(responseBytes),
	}).Debug("Received HTTP response")
	logger.Debugf("Response headers: %+v", resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx status code for target=%v path=%v: %v (len=%d)",
			arg.Target, arg.Path, resp.Status, len(responseBytes))
	}

	return json.Unmarshal(responseBytes, arg.Output)
}

func (dc *Conn) genericRequest(greq *genericRequest) (*genericResponse, error) {
	isOnline := dc.RequestMode && greq.requestIfOnline
	var part string
	if isOnline {
		part = "/app/res/request"
	} else {
		part = "/" + greq.Path
		greq.Path = ""
	}

	gresp := genericResponse{}
	err := dc.SimpleRequest(SimpleRequest{
		Path:   part,
		Input:  greq,
		Output: &gresp,
	})
	if err != nil {
		return nil, err
	}

	// fetch and append messages to queue (some are returned to us as part of this call)
	messages, err := gresp.Messages()
	if err != nil {
		return nil, err
	}
	for _, message := range messages {
		b, err := message.readData(dc.phoneSecret)
		if err != nil {
			return nil, err
		}

		logger.WithFields(logrus.Fields{
			"messageHeader": message,
			"decoded":       string(b),
		}).Debug("Got message from response")

		message.DecodedMessage = b

		if message.ProcessID == "" {
			dc.pendingMessages = append(dc.pendingMessages, message)
			continue
		}

		// Got an inline response
		if greq.ProcessID == message.ProcessID {
			if message.ProcessState == nil || *message.ProcessState == 0 {
				gresp.inlineResponse = b
				continue
			}
			// Not yet complete (ProcessState != 0), ignoring
			continue
		}

		// Check if we're part of the pending set
		dc.unresolvedMutex.Lock()
		prev, ok := dc.unresolvedRPC[message.ProcessID]
		if ok {
			delete(dc.unresolvedRPC, message.ProcessID)
			dc.unresolvedMutex.Unlock()
			prev <- message
			continue
		}
		dc.unresolvedMutex.Unlock()

		logger.Debugf("Dropping unknown response: %+v", message)
	}

	// fail if there's a server-reported error message
	if gresp.Message != "" {
		return nil, fmt.Errorf("got error message: %v", gresp.Message)
	}

	return &gresp, nil
}

func (dc *Conn) signedRequest(conf requestConfig) (*genericRequest, error) {
	sessionSig := newHubSignature(dc.sessionSecret)
	phoneSig := newHubSignature(dc.phoneSecretRaw)

	// Use local time or nextAccess time, whichever is greater
	localTime := int(time.Now().UnixNano() / 1e6)
	if localTime < dc.nextAccess {
		waitTime := time.Duration(dc.nextAccess-localTime) * time.Millisecond
		logger.WithField("waitTime", waitTime).Debug("Waiting until nextAccess")
		time.Sleep(waitTime)
	}

	// Update nextAccess after waiting
	localTime = int(time.Now().UnixNano() / 1e6)
	if localTime > dc.nextAccess {
		dc.nextAccess = localTime
	}
	dc.nextAccess += 1000 // Add one second, time in millis

	// Create an encrypted request
	c, err := NewEncCipher(dc.phoneSecret, dc.nextAccess)
	if err != nil {
		return nil, err
	}
	encData := base64.StdEncoding.EncodeToString(c.Encrypt(conf.data))

	dc.sequenceIDSuffix++ // Increment to track replies so process is unique
	greq := &genericRequest{
		ProcessID:        fmt.Sprintf("%s-%d", dc.processID, dc.sequenceIDSuffix),
		SessionID:        dc.sessionID,
		SessionSignature: sessionSig.Update(dc.nextAccess, encData),
		PhoneSignature:   phoneSig.Update(dc.nextAccess, encData),
		dataPayload: dataPayload{
			Time:        dc.nextAccess,
			Data:        encData,
			IsEncrypted: true,
		},
		Path:            conf.path,
		requestIfOnline: conf.requestIfOnline,
	}

	// Only need the BaseStation, not the rest of the credential
	greq.Credential.BaseStation = dc.cred.BaseStation

	logger.WithFields(logrus.Fields{
		"path":       conf.path,
		"processID":  greq.ProcessID,
		"nextAccess": dc.nextAccess,
	}).Debug("Generated signed request")

	dc.nextAccess = int(time.Now().UnixNano()/1e6) + 10000 // 10 seconds in milliseconds

	logger.WithField("nextAccess", dc.nextAccess).Debug("Next access time updated to 10 seconds later")

	return greq, nil
}

// Close shuts down this Conn.
func (dc *Conn) Close() {
	if dc.client != nil {
		dc.client.CloseIdleConnections()
		dc.client = nil
	}
}

// Connect passes credentials to the server and sets up secrets.
func (dc *Conn) Connect(cred Credential) error {
	// If dc.Debug == true, we allow Debug logs
	if dc.Debug {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	dc.cred = cred
	dc.unresolvedRPC = make(map[string]chan *Message)

	greq := &genericRequest{
		Credential:        cred,
		CommunicationType: 3, // 1 and 3 are valid
		Path:              "app/connect",
	}
	// The phoneSecret is not sent in the JSON body
	greq.Credential.PhoneSecret = ""

	// create 'random' processID
	now := time.Now()
	dc.processID = fmt.Sprintf("%d-E--%d", now.Unix(), now.Unix()*1e9-now.UnixNano())

	// Derive or store the phone secrets
	dc.phoneSecret = md5hash(cred.PhoneSecret)
	dc.phoneSecretRaw = []byte(cred.PhoneSecret)

	gresp, err := dc.genericRequest(greq)
	if err != nil {
		return err
	}

	crd := &connectResponseData{}
	if len(gresp.dataPayload.Data) == 0 {
		return errors.New("no valid payload from connect")
	}
	err = gresp.unmarshalData(dc.phoneSecret, crd)
	if err != nil {
		return err
	}

	dc.sessionID = gresp.SessionID
	dc.sessionSecret = []byte(gresp.SessionSecret)
	dc.nextAccess = crd.UserAccess.NextAccess

	// Example of structured logging with a single field "basicInfo"
	basicInfo := map[string]interface{}{
		"sessionID": dc.sessionID,
		"secret":    gresp.SessionSecret,
		"next":      crd.UserAccess.NextAccess,
	}
	logger.WithField("basicInfo", basicInfo).
		Info("Fetched basic information about the connection")

	return nil
}

// internalMessages does a messages poll, adding to any pending messages and resolving pending RPCs.
func (dc *Conn) internalMessages() error {
	dc.genericRequestMutex.Lock()
	defer dc.genericRequestMutex.Unlock()

	greq, err := dc.signedRequest(requestConfig{path: "app/res/messages"})
	if err != nil {
		return err
	}
	gresp, err := dc.genericRequest(greq)
	if err != nil {
		return err
	}

	messages, err := gresp.Messages()
	if err != nil {
		return err
	}

	logger.WithField("messageCount", len(messages)).Info("Fetched messages")
	
	for _, message := range messages {
		logger.WithField("processID", message.ProcessID).Info("Processing message")

		b, err := message.readData(dc.phoneSecret)
		if err != nil {
			logger.WithError(err).Error("Failed to decode message")
			continue
		}
		message.DecodedMessage = b

		if message.ProcessID != "" {
			dc.unresolvedMutex.Lock()
			if ch, exists := dc.unresolvedRPC[message.ProcessID]; exists {
				ch <- message
				delete(dc.unresolvedRPC, message.ProcessID)
			}
			dc.unresolvedMutex.Unlock()
		} else {
			dc.pendingMessages = append(dc.pendingMessages, message)
		}
	}

	return nil
}

// Messages gets any pending status messages from the server.
func (dc *Conn) Messages() ([]*Message, error) {
	if len(dc.pendingMessages) == 0 {
		if err := dc.internalMessages(); err != nil {
			return nil, err
		}
	}

	out := dc.pendingMessages
	dc.pendingMessages = nil
	return out, nil
}

type RPC struct {
	Path   string
	Input  interface{}
	Output interface{}
}

// Request makes a signed generic RPC and waits until its response is available.
func (dc *Conn) RPC(rpc RPC) error {
	var err error
	var b []byte

	if rpc.Input != nil {
		b, err = json.Marshal(rpc.Input)
		if err != nil {
			return err
		}
	}

	var path string
	if len(rpc.Path) > 0 {
		if rpc.Path[0] != '/' {
			return fmt.Errorf("rpc.Path must start with /, got: %v", rpc.Path)
		}
		path = rpc.Path[1:]
	}

	// Wrap sign/send in inner fn so we can lock while it occurs.
	resp, pid, err := func() (*genericResponse, string, error) {
		dc.genericRequestMutex.Lock()
		defer dc.genericRequestMutex.Unlock()

		greq, err := dc.signedRequest(requestConfig{data: b, path: path, requestIfOnline: true})
		if err != nil {
			return nil, "", err
		}

		resp, err := dc.genericRequest(greq)
		return resp, greq.ProcessID, err
	}()
	if err != nil {
		return err
	}

	logger.WithField("resp", resp).Debug("RPC resp")
	var responseBytes []byte
	if resp.inlineResponse != nil {
		responseBytes = resp.inlineResponse
	} else {
		responseBytes, err = dc.waitForPid(pid)
		if err != nil {
			return err
		}
	}

	// Unmarshal to see if we got a code != 0
	var output struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
	}
	err = json.Unmarshal(responseBytes, &output)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"rawInlineResponse": string(responseBytes),
			"error":             err,
		}).Error("Could not decode non-JSON response")

		return err
	}
	if output.Code != 0 {
		return fmt.Errorf("got unhandled error calling path=%v code=%v note=%v",
			rpc.Path, output.Code, output.Description)
	}

	if rpc.Output != nil {
		return json.Unmarshal(responseBytes, rpc.Output)
	}
	return nil
}

// waitForPid waits for the server to respond with a matching processID.
func (dc *Conn) waitForPid(pid string) ([]byte, error) {
	ch := make(chan *Message, 1) // Buffered to avoid blocking
	dc.unresolvedMutex.Lock()
	dc.unresolvedRPC[pid] = ch
	dc.unresolvedMutex.Unlock()

	logger.WithField("pid", pid).Debug("Waiting for process response")

	defer func() {
		// Clean up unresolvedRPC to avoid stale entries
		dc.unresolvedMutex.Lock()
		delete(dc.unresolvedRPC, pid)
		dc.unresolvedMutex.Unlock()
	}()

	timeout := time.NewTimer(time.Minute)
	defer timeout.Stop()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case m := <-ch:
			logger.WithField("pid", pid).Debug("Received process response")
			return m.DecodedMessage, nil

		case <-tick.C:
			logger.WithField("pid", pid).Debug("Polling messages for response")
			if err := dc.internalMessages(); err != nil {
				logger.WithError(err).WithField("pid", pid).Error("Error fetching messages")
				return nil, err
			}

		case <-timeout.C:
			logger.WithField("pid", pid).Warn("Timeout waiting for process response")
			return nil, ErrTimeout
		}
	}
}
