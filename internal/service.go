package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ConfigPath = "./config"

type dataObserver struct {
	folders []string
}

type appConfig struct {
	storageCreds map[string]string
	Folders      []string `json:"folders"`
}

func getConfig() (*appConfig, error) {
	path := filepath.Join(ConfigPath, "config.json")
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("file with given path is not exist %s", path)
		}
		return nil, err
	}

	decoder := json.NewDecoder(f)
	if decoder == nil {
		return nil, errors.New("couldn't initialize decoder")
	}

	var config appConfig

	for decoder.More() {
		err := decoder.Decode(&config)
		if err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func (observer *dataObserver) start() {
	for {
		fmt.Println("do shit")
		time.Sleep(1 * time.Second)
	}
}

func Start() {
	config, err := getConfig()
	if err != nil {
		panic(err)
	}
	observer := dataObserver{
		folders: config.Folders,
	}

	go observer.start()
}
