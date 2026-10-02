package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/joho/godotenv"
)

// Dir is george's config directory: GEORGE_CONFIG_DIR, else
// $XDG_CONFIG_HOME/george. It holds env, mcp.toml, PERSONA.md, and SELF.md.
func Dir() string {
	if d := strings.TrimSpace(os.Getenv("GEORGE_CONFIG_DIR")); d != "" {
		return d
	}
	return filepath.Join(xdg.ConfigHome, "george")
}

// DataHome is the default DATA_DIR: $XDG_DATA_HOME/george.
func DataHome() string {
	return filepath.Join(xdg.DataHome, "george")
}

// BinDir is where tools-fetch puts the MCP binaries by default.
func BinDir() string {
	return filepath.Join(DataHome(), "bin")
}

// EnvFile is the env file Load reads before the process env.
func EnvFile() string {
	return filepath.Join(Dir(), "env")
}

// loadEnvFile sets every variable in EnvFile that the process env doesn't
// already set. A missing file is fine.
func loadEnvFile() error {
	path := EnvFile()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("env file %s: %w", path, err)
	}
	return nil
}
