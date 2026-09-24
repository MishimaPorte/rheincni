package main

import (
	"C"
)
import (
	"encoding/json"
	"io"
	"os"
	"rheincni"
)

var envConfig rheincni.EnvConfiguration
var cniConfig rheincni.CniConfiguration

func ProcessError(err error, out io.Writer) {

}

func main() {
	err := rheincni.ParseEnv(&envConfig)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}

	err = rheincni.ParseJsonInput(os.Stdin, &cniConfig)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}

	var result rheincni.Result
	result.CniVersion = "1.0.0"

	switch envConfig.Command {
	case rheincni.CNICommand_ADD:
		err = rheincni.Add(&envConfig, &cniConfig)
		ProcessError(err, os.Stderr)
		os.Exit(1)
	case rheincni.CNICommand_DEL:
		panic("TODO")
	case rheincni.CNICommand_CHECK:
		panic("TODO")
	case rheincni.CNICommand_VERSION:
		panic("TODO")
	default:
		panic("UNREACHABLE")
	}

	err = json.NewEncoder(os.Stdout).Encode(&result)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}
}
