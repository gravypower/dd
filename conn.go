package dd

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var (
	// ErrNotConnected is returned if Conn isn't set up properly.
	ErrNotConnected = errors.New("not connected")
)

// Conn is a connection to the service.
type Conn struct {
	Version            string // version number to send
	Target             string // base URL
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
	Phone             string `json:"phoneId,omitempty"`
	PhonePassword     string `json:"phonePassword,omitempty"`
	UserPassword      string `json:"userPassword,omitempty"`
	CommunicationType int    `json:"communicationType,omitempty"`
}

type genericResponse struct {
	dc         *Conn // for knowledge of Debug
	statusCode int

	SessionSignature string `json:"sessionSig"`
	RawMessages      string `json:"messages"`
	Message          string `json:"message"`
	BaseStation      string `json:"bsid"`
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

func (dc *Conn) request(greq *genericRequest) (*genericResponse, error) {
	b, err := json.Marshal(greq)
	if err != nil {
		return nil, err
	}

	isOnline := dc.RequestMode && greq.requestIfOnline
	part := greq.Path
	if isOnline {
		part = "app/res/request"
	} else {
		greq.Path = ""
	}

	buf := bytes.NewBuffer(b)
	url := fmt.Sprintf("%s/%s", dc.Target, part)
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return nil, err
	}
	if dc.Debug {
		log.Printf("sending url=%v bytes: %v", url, string(b))
	}

	req.Header.Set("User-Agent", fmt.Sprintf("sddAndroid-%s-LGE Nexus 5X(28)", dc.Version))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("version", dc.Version)

	if dc.client == nil {
		return nil, ErrNotConnected // need to call .Connect first
	}
	resp, err := dc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// nb. we could use json.NewDecoder(..) here, but this way logging bytes is easy
	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if dc.Debug {
		log.Printf("got raw response: (code=%v) %v", resp.StatusCode, string(responseBytes))
		log.Printf("got response headers: %+v", resp.Header)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("non-2xx status code for %v: %v (len=%d)", part, resp.Status, len(responseBytes))
	}

	gresp := &genericResponse{
		dc:         dc,
		statusCode: resp.StatusCode,
	}
	err = json.Unmarshal(responseBytes, gresp)
	if err != nil {
		log.Printf("failed to unmarshal: %v", string(responseBytes))
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
		dc.pendingMessages = append(dc.pendingMessages, message)
	}

	// fail if there's a message
	if gresp.Message != "" {
		return nil, fmt.Errorf("got error message: %v", gresp.Message)
	}

	return gresp, nil
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
		// nb. dc.client.CloseIdleConnections() appears in Go 1.12
		if t, ok := dc.client.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
		dc.client = nil
	}
}

// Connect passes credentials to the server and sets up secrets.
func (dc *Conn) Connect(cred Credential) error {
	dc.Close()

	// append cert to the pool for our transport
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if ok := rootCAs.AppendCertsFromPEM(selfSignedPEM); !ok {
		return errors.New("could not parse included PEM")
	}

	// setup Client that allows updated CAs
	dc.client = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:            rootCAs,
				InsecureSkipVerify: dc.InsecureSkipVerify,
			},
		},
	}

	dc.cred = cred
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

	gresp, err := dc.request(greq)
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

// Messages gets any pending messages from the server.
func (dc *Conn) Messages(checkIfNone bool) ([]*Message, error) {
	if len(dc.pendingMessages) == 0 && checkIfNone {
		greq, err := dc.signedRequest(requestConfig{path: "app/res/messages"})
		if err != nil {
			return nil, err
		}
		_, err = dc.request(greq)
		if err != nil {
			return nil, err
		}
	}

	out := dc.pendingMessages
	dc.pendingMessages = nil
	return out, nil
}

// Request makes a generic request and returns the ID of the request.
func (dc *Conn) Request(path string, payload interface{}) (string, error) {
	var err error
	var b []byte

	if payload != nil {
		b, err = json.Marshal(payload)
		if err != nil {
			return "", err
		}
	}

	// create/sign request
	greq, err := dc.signedRequest(requestConfig{data: b, path: path, requestIfOnline: true})
	if err != nil {
		return "", err
	}

	resp, err := dc.request(greq)
	if err != nil {
		return "", err
	}

	_, err = resp.dataPayload.readData(dc.phoneSecret)
	return greq.ProcessID, err
}
