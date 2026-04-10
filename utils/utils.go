package utils

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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

// Takes input as pointer to configuration structure
// with exportable JSON fields and 'prompt' tags
func ConfigPromptInitialize(configuration any) error {
	refVal := reflect.ValueOf(configuration).Elem()
	refT := refVal.Type()

	for i := range refT.NumField() {
		field := refT.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}

		prompt := field.Tag.Get("prompt")
		if len(prompt) == 0 {
			continue
		}
		slog.Info(prompt)
		input, err := GetInput(prompt)
		if err != nil {
			return err
		}

		fieldVal := refVal.FieldByName(field.Name)
		if fieldVal.IsValid() != true {
			return fmt.Errorf("error occured during setting fields %s field on %s struct is not valid", field.Name, refT.Name())
		}
		fieldVal.Set(reflect.ValueOf(input))
	}
	return nil
}
