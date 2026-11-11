package config

type Config struct {
	Containerlab struct {
		BinPath string `yaml:"bin_path"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"containerlab"`
	Theme    string `yaml:"theme"`
	Topology struct {
		AutoRefresh bool `yaml:"auto_refresh"`
	} `yaml:"topology"`
}

func Default() Config {
	var c Config
	c.Theme = "default"
	c.Containerlab.Timeout = 300
	c.Topology.AutoRefresh = true
	return c
}
