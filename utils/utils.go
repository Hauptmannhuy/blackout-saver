package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var basePath string = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("couldn't resolve home path %w", err))
	}

	return filepath.Join(home, ".ssh")

}()

func GetSSHFile(name string) (*os.File, error) {
	file, err := os.Open(filepath.Join(basePath, name))
	if err != nil {
		return nil, fmt.Errorf("couldn't open file %w", err)
	}
	return file, nil
}

func ReadSSHFile(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(basePath, name))
}

func GetInput(prompt ...string) (string, error) {
	desc := "enter input..."
	if prompt == nil {
		desc = strings.Join(prompt, " ")
	}
	var input string
	if _, err := fmt.Print(desc); err != nil {
		return "", err
	}
	if _, err := fmt.Scan(&input); err != nil {
		return "", err
	}
	return strings.Trim(input, " "), nil
}
