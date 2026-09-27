package rheincni

import (
	"encoding/json"
	"io"
)

type Interface struct {
	Name    string `json:"name"`
	Mac     Mac    `json:"mac"`
	Sandbox string `json:"sandbox,omitemtpy"`
}
type IPConfig struct {
	Address   IPSubnet `json:"address"`
	Gateway   IP       `json:"gateway"`
	Interface int      `json:"interface"`
}
type Route struct {
	Dst IPSubnet `json:"dst"`
	Gw  IP       `json:"gw"`
}

type Result struct {
	CniVersion string
	Interfaces []Interface
	Ips        []IPConfig
	Routes     []Route
}

type ErrorKind int

const (
	ErrorKind_IncompatVersion      ErrorKind = 1
	ErrorKind_UnsupportedConfig    ErrorKind = 2
	ErrorKind_UnknownContainer     ErrorKind = 3
	ErrorKind_InvalidEnv           ErrorKind = 4
	ErrorKind_IOFailure            ErrorKind = 5
	ErrorKind_FailedToDecode       ErrorKind = 6
	ErrorKind_InvalidNetworkConfig ErrorKind = 7
	ErrorKind_TransientTryAgain    ErrorKind = 11
)

type ErrorResult struct {
	CniVersion string    `json:"version"`
	Msg        string    `json:"msg"`
	Details    string    `json:"details"`
	Code       ErrorKind `json:"code"`
}

func ProcessError(err error, cniVersion string, outErrResult *ErrorResult, out io.Writer) {
	if outErrResult == nil {
		outErrResult = new(ErrorResult)
	}
	outErrResult.CniVersion = cniVersion
	outErrResult.Code = ErrorKind_IOFailure
	outErrResult.Msg = err.Error()
	outErrResult.Details = err.Error()
	if out != nil {
		err := json.NewEncoder(out).Encode(outErrResult)
		if err != nil {
			panic(err.Error())
		}
	}
}
