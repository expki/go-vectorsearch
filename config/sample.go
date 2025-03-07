package config

import (
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/expki/go-vectorsearch/env"
)

// CreateSample creates a sample configuration file.
func CreateSample(path string) error {
	sample := Config{
		Server: ConfigServer{
			HttpAddress:  ":7500",
			HttpsAddress: ":7501",
		},
		TLS: ConfigTLS{
			DomainNameServer: []string{},
			IP:               []string{},
			Certificates:     []*ConfigTLSPath{},
		},
		Ollama: Ollama{
			Url:      []string{"http://localhost:11434"},
			Embed:    "nomic-embed-text",
			Generate: "llama3.2",
			Chat:     "llama3.2",
		},
		Database: Database{
			Sqlite: "./vectors.db",
		},
		Cache: "./cache/",
	}
	raw, err := json.MarshalIndent(sample, "", "    ")
	if err != nil {
		return fmt.Errorf("could not marshal sample config: %v", err)
	}
	err = os.WriteFile(path, raw, 0600)
	if err != nil {
		return fmt.Errorf("could not write sample config file: %v", err)
	}
	return nil
}
