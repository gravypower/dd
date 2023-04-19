package dd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	RemoteAPIBase = "https://version2.smartdoordevices.com"
	SDKPort       = 8991
	DefaultPort   = 8989
)

// Conn is a connection to the service.
type Conn struct {
	Version            string // version number to send
	Host               string // hostname
	InsecureSkipVerify bool   // ignore certs, used for local conns
	RequestMode        bool   // whether to "request" changes, used for talking to online server
	Debug              bool   // whether to log debug

	cred   Credential   // cached creds
	client *http.Client // cached optional client

	processID      string // random process ID to use in requests
	sessionID      string // session ID returned from server
	nextAccess     int    // the next timestamp to use (millis)
	sessionSecret  []byte // to calculate sessionSignature, from server
	phoneSecret    []byte // to calculate phoneSignature, specified locally (md5 of raw)
	phoneSecretRaw []byte // raw secret, UTF-8 bytes of string

	sequenceIDSuffix int // incremented suffix (to track replies)
	pendingMessages  []*Message

	unresolvedMutex sync.Mutex
	unresolvedRPC   map[string]chan *Message
}

// Credential holds login/connect credentials.
type Credential struct {
	BaseStation   string // base station ID
	PhoneSecret   string // phone secret
	Phone         string // phone ID
	PhonePassword string // phone password
	UserPassword  string // user password
}

type requestConfig struct {
	data            []byte
	path            string
	requestIfOnline bool // does this need to be "requested" via /app/res/request
}

type genericRequest struct {
	requestIfOnline bool // does this need to be "requested" via /app/res/request
	dataPayload

	BaseStation      string `json:"bsid"`
	SessionID        string `json:"sessionId,omitempty"`
	ProcessID        string `json:"processId,omitempty"`
	SessionSignature string `json:"sessionSig,omitempty"`
	PhoneSignature   string `json:"phoneSig,omitempty"`
	Path             string `json:"path,omitempty"`

	// connect only
	UserId            string `json:"userId,omitempty"`
	Phone             string `json:"phoneId,omitempty"`
	PhonePassword     string `json:"phonePassword,omitempty"`
	UserPassword      string `json:"userPassword,omitempty"`
	CommunicationType int    `json:"communicationType,omitempty"`
}

type genericResponse struct {
	SessionSignature string `json:"sessionSig"`
	RawMessages      string `json:"messages"`
	Message          string `json:"message"`
	BaseStation      string `json:"bsid"`
	inlineResponse   []byte
	dataPayload

	// connect response
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
	ProcessState   *int   `json:"processState"` // nb. sometimes is unset, vs 0
	PhoneSignature string `json:"phoneSig"`
	Type           int    `json:"type"`
	dataPayload

	DecodedMessage []byte `json:"-"` // actual decoded message
}

func (m *Message) Decode(target interface{}) error {
	return json.NewDecoder(bytes.NewBuffer(m.DecodedMessage)).Decode(target)
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

// Performs a simple request to our device. Does not care about sessions.
func (dc *Conn) SimpleRequest(arg SimpleRequest) error {
	if len(arg.Path) > 0 && arg.Path[0] != '/' {
		return fmt.Errorf("path must start with /, got: %v", arg.Path)
	}

	jsonBytes, err := json.Marshal(arg.Input)
	if err != nil {
		return err
	}

	var url string
	if arg.Target == RemoteTarget {
		url = fmt.Sprintf("https://%s%s", RemoteAPIBase, arg.Path)
	} else if arg.Target == SDKTarget {
		url = fmt.Sprintf("https://%s:%d%s", dc.Host, SDKPort, arg.Path)
	} else if arg.Target == DefaultTarget {
		url = fmt.Sprintf("https://%s:%d%s", dc.Host, DefaultPort, arg.Path)
	} else {
		return fmt.Errorf("unknown target: %v", arg.Target)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	if dc.Debug {
		log.Printf("sending url=%v json=%v", url, string(jsonBytes))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("sddAndroid-%s-LGE Nexus 5X(28)", dc.Version))
	req.Header.Set("version", dc.Version)
	req.Header.Set("platform", "android")

	// Implicitly create unauthenticated client.
	if dc.client == nil {
		customTransport := http.DefaultTransport.(*http.Transport).Clone()
		customTransport.TLSClientConfig.InsecureSkipVerify = true
		dc.client = &http.Client{Transport: customTransport}
	}

	resp, err := dc.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// nb. we could use json.NewDecoder(..) here, but this way logging bytes is easy
	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if dc.Debug {
		log.Printf("got raw response: (code=%v) %v", resp.StatusCode, string(responseBytes))
		log.Printf("got response headers: %+v", resp.Header)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx status code for target=%v path=%v: %v (len=%d)", arg.Target, arg.Path, resp.Status, len(responseBytes))
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
		if dc.Debug {
			log.Printf("got message: %+v %+v", message, string(b))
		}
		message.DecodedMessage = b

		// Got an inline response
		if greq.ProcessID == message.ProcessID {
			if message.ProcessState == nil || *message.ProcessState == 0 {
				gresp.inlineResponse = b
				continue
			}
			// We're ignoring this because it's not yet complete
			// TODO: check that the string makes sense?
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
		dc.pendingMessages = append(dc.pendingMessages, message)
	}

	// fail if there's a message (this is not "messages", but rather, an error)
	if gresp.Message != "" {
		return nil, fmt.Errorf("got error message: %v", gresp.Message)
	}

	return &gresp, nil
}

func (dc *Conn) signedRequest(conf requestConfig) (*genericRequest, error) {
	sessionSig := newHubSignature(dc.sessionSecret)
	phoneSig := newHubSignature(dc.phoneSecretRaw)

	// use localTime or nextAccess time, whichever is greater
	dc.nextAccess += 1000 // add one second, time in millis
	localTime := int(time.Now().UnixNano() / 1e6)
	if localTime > dc.nextAccess {
		dc.nextAccess = localTime
	}
	nextAccess := dc.nextAccess

	c, err := NewEncCipher(dc.phoneSecret, nextAccess)
	if err != nil {
		return nil, err
	}
	encData := base64.StdEncoding.EncodeToString(c.Encrypt(conf.data))

	dc.sequenceIDSuffix++ // increment to track replies so process is unique
	greq := &genericRequest{
		BaseStation:      dc.cred.BaseStation,
		ProcessID:        fmt.Sprintf("%s-%d", dc.processID, dc.sequenceIDSuffix),
		SessionID:        dc.sessionID,
		SessionSignature: sessionSig.Update(nextAccess, encData),
		PhoneSignature:   phoneSig.Update(nextAccess, encData),
		dataPayload: dataPayload{
			Time:        nextAccess,
			Data:        encData,
			IsEncrypted: true,
		},
		Path:            conf.path,
		requestIfOnline: conf.requestIfOnline,
	}
	if dc.Debug {
		log.Printf("generated req for path=%s id=%s", conf.path, greq.ProcessID)
	}
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
	dc.cred = cred
	dc.unresolvedRPC = make(map[string]chan *Message)

	greq := &genericRequest{
		BaseStation:       cred.BaseStation,
		Phone:             cred.Phone,
		PhonePassword:     cred.PhonePassword,
		UserPassword:      cred.UserPassword,
		CommunicationType: 3, // 1 and 3 are valid
		Path:              "app/connect",
	}

	// create 'random' processID
	now := time.Now()
	dc.processID = fmt.Sprintf("%d-E--%d", now.Unix(), now.Unix()*1e9-now.UnixNano())
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

	return nil
}

// Messages gets any pending status messages from the server.
func (dc *Conn) Messages() ([]*Message, error) {
	if len(dc.pendingMessages) == 0 {
		greq, err := dc.signedRequest(requestConfig{path: "app/res/messages"})
		if err != nil {
			return nil, err
		}
		_, err = dc.genericRequest(greq)
		if err != nil {
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
// In some cases this might rely on Messages() being called occasionally.
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

	// create/sign request
	greq, err := dc.signedRequest(requestConfig{data: b, path: path, requestIfOnline: true})
	if err != nil {
		return err
	}

	resp, err := dc.genericRequest(greq)
	if err != nil {
		return err
	}

	var responseBytes []byte

	if resp.inlineResponse != nil {
		responseBytes = resp.inlineResponse
	} else {
		ch := make(chan *Message)
		dc.unresolvedMutex.Lock()
		dc.unresolvedRPC[greq.ProcessID] = ch
		dc.unresolvedMutex.Unlock()
		if dc.Debug {
			log.Printf("! Delaying for process=%v", greq.ProcessID)
		}
		m := <-ch
		responseBytes = m.DecodedMessage
	}

	// Unmarshal into generic response to see if we were a valid command
	var output struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
	}
	err = json.Unmarshal(responseBytes, &output)
	if err != nil {
		if dc.Debug {
			log.Printf("could not decode non-json: %v", string(resp.inlineResponse))
		}
		return err
	}
	if output.Code != 0 {
		return fmt.Errorf("got unhandled error calling path=%v code=%v note=%v", rpc.Path, output.Code, output.Description)
	}

	return json.Unmarshal(responseBytes, rpc.Output)
}
