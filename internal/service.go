package internal

import (
	"blackout-saver/transport"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	uploader transport.Uploader
	watcher  *fsnotify.Watcher
	config   *AppConfig
}

type AppConfig struct {
	Folders      []string                  `json:"folders"`
	Files        []string                  `json:"files"`
	TransportCfg transport.TransportConfig `json:"transport"`
	jsonFile     *os.File
}

func getConfig() (*AppConfig, error) {
	path := filepath.Join(ConfigPath, "config.json")
	jsonFile, err := getConfigJSONFile(path)

	config := &AppConfig{
		jsonFile: jsonFile,
	}

	if config.TransportCfg == nil {
		config.TransportCfg = &transport.TransportConfigBase{
			TransportType: "",
		}
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

func newfileWatcher(config *AppConfig) (*fileWatcher, error) {
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

func Start() {
	config, err := getConfig()
	if err != nil {
		panic(err)
	}
	fileWatcher, err := newfileWatcher(config)

	log.Fatal(err)
	fileWatcher.start()
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

func (config *AppConfig) saveToJSON(file *os.File) error {
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
func SetTransport() error {
	config, err := getConfig()
	if err != nil {
		panic(err)
	}
	transportCfg, err := transport.NewTransportConfig()
	if err != nil {
		panic(err)
	}
	config.TransportCfg = transportCfg
	return config.saveToJSON(config.jsonFile)
}

// TODO: think about how to implement uploading data locally with SSH
