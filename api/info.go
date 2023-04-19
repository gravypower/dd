package api

type BasicInfo struct {
	BaseStation string `json:"bsid"`
	Mono        int64  `json:"mono"`
	Clock       int64  `json:"clock"`
	Name        string `json:"name"`
	Version     int    `json:"version"`
}
