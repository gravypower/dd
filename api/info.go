package api

import "github.com/gravypower/dd"

type BasicInfo struct {
	BaseStation string `json:"bsid"`
	Mono        int64  `json:"mono"`
	Clock       int64  `json:"clock"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
}

// FetchBasicInfo fetches basic device information and returns an error if it fails.
// This function no longer calls Fatal() to allow graceful error handling.
func FetchBasicInfo(conn *dd.Conn) (*BasicInfo, error) {
	var info BasicInfo
	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		logger.WithError(err).Error("could not get basic info")
		return nil, err
	}
	return &info, nil
}
