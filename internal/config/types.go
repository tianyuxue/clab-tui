package config

type Config struct {
	Containerlab struct {
		BinPath string `yaml:"bin_path"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"containerlab"`
	Theme string `yaml:"theme"`
}

func Default() Config {
	var c Config
	c.Theme = "default"
	c.Containerlab.Timeout = 300
	return c
}
