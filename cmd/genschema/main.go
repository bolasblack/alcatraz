package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/invopop/jsonschema"

	"github.com/bolasblack/alcatraz/internal/config"
)

func main() {
	r := jsonschema.Reflector{
		// Use toml tag for property names since config is for .alca.toml files
		FieldNameTag:             "toml",
		RequiredFromJSONSchemaTags: true,
	}

	schema := r.Reflect(&config.Config{})
	schema.Title = "Alcatraz Configuration"
	schema.Description = "Configuration schema for .alca.toml"
	schema.ID = ""

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		if err := os.WriteFile(os.Args[1], data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println(string(data))
	}
}
