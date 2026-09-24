package rheincni

type Interface struct {
	Name    string `json:"name"`
	Mac     Mac    `json:"mac"`
	Sandbox string `json:"sandbox,omitemtpy"`
}
type IPConfig struct {
	Address   IP  `json:"address"`
	Gateway   IP  `json:"gateway"`
	Interface int `json:"interface"`
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
