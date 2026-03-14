package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/fsnotify/fsnotify"
)

const (
	ConfigPath = "./config"
	cfgName    = ConfigPath + "/" + "config.json"
)

type fileWatcher struct {
	watcher *fsnotify.Watcher
	config  *appConfig
}

type appConfig struct {
	Folders      []string         `json:"folders"`
	Files        []string         `json:"files"`
	TransportCfg *transportConfig `json:"transport"`
	jsonFile     *os.File
}

type transportConfig struct {
	TransportType string `json:"type"`
}

func getConfig() (*appConfig, error) {
	path := filepath.Join(ConfigPath, "config.json")
	jsonFile, err := getConfigJSONFile(path)
	config := &appConfig{
		jsonFile: jsonFile,
	}

	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(jsonFile)
	if decoder == nil {
		return nil, errors.New("couldn't initialize decoder")
	}

	for decoder.More() {
		err := decoder.Decode(&config)
		if err != nil {
			return nil, err
		}
	}

	if config.TransportCfg == nil {
		config.TransportCfg = &transportConfig{
			TransportType: "",
		}
	}

	fmt.Print(*config)
	return config, nil
}

func (fileWatcher *fileWatcher) start() {
	for event := range fileWatcher.watcher.Events {
		path := event.Name
		if event.Has(fsnotify.Write) {
			fmt.Printf("%s file was modified", path)
		}
	}
}

func newfileWatcher(config *appConfig) (*fileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fileWatcher := &fileWatcher{
		config:  config,
		watcher: watcher,
	}
	for _, v := range append(config.Files, config.Folders...) {
		err := watcher.Add(v)
		if err != nil {
			return nil, err
		}
	}
	return fileWatcher, nil
}

func Start() error {
	config, err := getConfig()
	if err != nil {
		panic(err)
	}
	fileWatcher, err := newfileWatcher(config)
	if err != nil {
		return err
	}
	go fileWatcher.start()
	return nil
}

func AddDirToWatch(filePath string) error {
	var containerToAppend *[]string
	config, err := getConfig()
	cfgFile := config.jsonFile

	if err != nil {
		return err
	}
	fInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("error during file %s validation, err %s", filePath, err.Error())
	}

	containerToAppend = &config.Folders
	if !fInfo.IsDir() {
		containerToAppend = &config.Files
	}

	if slices.Contains(*containerToAppend, filePath) {
		return fmt.Errorf("%s already in config", filePath)
	}
	*containerToAppend = append(*containerToAppend, filePath)

	return config.saveToJSON(cfgFile)
}

func (config *appConfig) saveToJSON(file *os.File) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	_, err = file.WriteAt(data, 0)
	if err != nil {
		return err
	}

	return file.Close()
}

func getConfigJSONFile(filepath string) (*os.File, error) {
	var f *os.File
	_, err := os.Stat(filepath)
	if err != nil && os.IsNotExist(err) {
		var createErr error
		f, createErr = os.OpenFile(filepath, os.O_RDWR|os.O_CREATE, 0644)
		if createErr != nil {
			return nil, errors.Join(err, createErr)
		}
		return f, nil

	} else {
		return os.OpenFile(filepath, os.O_RDWR, 0644)
	}
}

// right now support for sftp is planning
func SetTransport(transport string) error {
	config, err := getConfig()
	if err != nil {
		return err
	}
	config.TransportCfg.TransportType = transport
	return config.saveToJSON(config.jsonFile)
}

// TODO: think about how to implement uploading data locally with SSH
