package dd

import (
	"net/http"
	"sync"
)

type SimpleRequestTarget int

type SimpleRequest struct {
	Path   string              // Path for request
	Target SimpleRequestTarget // Where to call
	Input  interface{}
	Output interface{}
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

type RPC struct {
	Path   string
	Input  interface{}
	Output interface{}
}
