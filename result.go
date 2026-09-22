package main

type Interface struct {
	Name    string `json:"name"`
	Sandbox string `json:"sandbox,omitemtpy"`
}
type Ip struct {
	Address   string `json:"address"`
	Gateway   string `json:"gateway"`
	Interface int    `json:"interface"`
}
type Route struct {
	Dst string `json:"dst"`
	Gw  string `json:"gw"`
}

type Result struct {
	CniVersion string
	Interfaces []Interface
	Ips        []Ip
	Routes     []Route
}
