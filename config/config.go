package config

import (
	"encoding/json"
	"fmt"

	_ "github.com/expki/govecdb/env"
)

// ParseConfig parses the raw JSON configuration.
func ParseConfig(raw []byte) (config Config, err error) {
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return config, fmt.Errorf("unmarshal config: %v", err)
	}
	return config, nil
}

type Config struct {
	Server   ConfigServer `json:"server"`
	TLS      ConfigTLS    `json:"tls"`
	Database Database     `json:"database"`
	Ollama   Ollama       `json:"ollama"`
}

type ConfigServer struct {
	HttpAddress  string `json:"http_address"`
	HttpsAddress string `json:"https_address"`
}
