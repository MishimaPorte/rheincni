package rheincni

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type CNICommand int

const (
	CNICommand_ADD CNICommand = iota
	CNICommand_DEL
	CNICommand_CHECK
	CNICommand_VERSION
)

type EnvConfiguration struct {
	Command     CNICommand
	ContainerId string
	Netns       string
	Ifname      string
	Args        string
	CniPath     string
}

func (c *CNICommand) String() string {
	switch *c {
	case CNICommand_ADD:
		return "ADD"
	case CNICommand_DEL:
		return "DEL"
	case CNICommand_CHECK:
		return "CHECK"
	case CNICommand_VERSION:
		return "VERSION"
	default:
		return fmt.Sprintf("unknown cni command: %d", *c)
	}
}

func ParseEnv(c *EnvConfiguration) error {
	command := os.Getenv("CNI_COMMAND") // ADD, DEL, CHECK, or VERSION
	switch command {
	case "ADD":
		c.Command = CNICommand_ADD
	case "DEL":
		c.Command = CNICommand_DEL
	case "CHECK":
		c.Command = CNICommand_CHECK
	case "VERSION":
		c.Command = CNICommand_VERSION
	default:
		return fmt.Errorf("unknown cni command: %s", command)
	}
	c.ContainerId = os.Getenv("CNI_CONTAINERID") // Runtime-assigned container or sandbox ID
	c.Netns = os.Getenv("CNI_NETNS")             // Path to the pod’s network namespace
	c.Ifname = os.Getenv("CNI_IFNAME")           // Interface name to configure inside it, usually eth0
	c.Args = os.Getenv("CNI_ARGS")               // Optional KEY=value;KEY2=value string
	c.CniPath = os.Getenv("CNI_PATH")            // Optional search path for delegated CNI binaries
	return nil
}

type CniConfiguration struct {
	CniVersion string `json:"cniVersion"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

func ParseJsonInput(r io.Reader, c *CniConfiguration) error {
	bts, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	os.Stderr.Write(bts)
	return json.NewDecoder(bytes.NewReader(bts)).Decode(c)
}
