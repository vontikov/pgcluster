package integration

import (
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

const defaultLoggingLevel = "info"

type Logging struct {
	Level string `yaml:"level"`
}

type Port struct {
	Name      string `yaml:"name"`
	Published int    `yaml:"published"`
	Target    int    `yaml:"target"`
}

type Container struct {
	Image    string   `yaml:"image"`
	Name     string   `yaml:"container_name"`
	Hostname string   `yaml:"hostname"`
	Command  string   `yaml:"command"`
	Env      []string `yaml:"environment"`
	Ports    []Port   `yaml:"ports"`
	Volumes  []string `yaml:"volumes"`
}

type Config struct {
	Logging   *Logging     `yaml:"logging"`
	Container []*Container `yaml:"SUT"`
}

func NewConfig(path string) (cfg *Config, err error) {
	var b []byte
	if b, err = ioutil.ReadFile(path); err != nil {
		return
	}

	c := &Config{}
	if err = c.parse(b); err != nil {
		return
	}
	return c, nil
}

func (c *Config) parse(b []byte) (err error) {
	if err = yaml.Unmarshal(b, c); err != nil {
		return
	}
	if c.Logging == nil {
		c.Logging = &Logging{Level: defaultLoggingLevel}
	}
	return
}
