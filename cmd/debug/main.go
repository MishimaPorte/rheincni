package main

import (
	"fmt"
	"rheincni"
)

func main() {
	var mp rheincni.MacPair
	rheincni.GenerateRandomMacPair(&mp)
	fmt.Println(mp)
	err := rheincni.CreateVethPeer("aboba", "aboba", "/var/run/netns/my_ns", mp)
	fmt.Println(err)
}
