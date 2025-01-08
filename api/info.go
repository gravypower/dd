package api

import "github.com/samthor/dd"

type BasicInfo struct {
	BaseStation string `json:"bsid"`
	Mono        int64  `json:"mono"`
	Clock       int64  `json:"clock"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
}

func FetchBasicInfo(conn *dd.Conn) BasicInfo {
	var info BasicInfo
	err := conn.SimpleRequest(dd.SimpleRequest{
		Path:   "/sdk/info",
		Target: dd.SDKTarget,
		Output: &info,
	})
	if err != nil {
		logger.WithError(err).Fatalf("could not get basic info")
	}
	return info
}
