package main

import (
	"C"
)
import (
	"encoding/json"
	"io"
	"os"
)

var envConfig EnvConfiguration
var cniConfig CniConfiguration

func ProcessError(err error, out io.Writer) {

}

func main() {
	err := ParseEnv(&envConfig)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}

	err = ParseJsonInput(os.Stdin, &cniConfig)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}

	var result Result
	result.CniVersion = "1.0.0"

	err = json.NewEncoder(os.Stdout).Encode(&result)
	if err != nil {
		ProcessError(err, os.Stderr)
		os.Exit(1)
	}
}
