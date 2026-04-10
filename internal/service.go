package internal

import (
	"blackout-saver/transport"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	ConfigPath     = "./config"
	configFileName = "client_config.json"
)

type fileWatcher struct {
	uploader transport.Uploader
	watcher  *fsnotify.Watcher
	config   *AppConfig
	pool     *workerPool
}

type AppConfig struct {
	Folders      []string                  `json:"folders"`
	Files        []string                  `json:"files"`
	TransportCfg transport.TransportConfig `json:"transport"`
	jsonFile     *os.File
}

type workerPool struct {
	tasks chan string
}

func newfileWatcher(config *AppConfig) (*fileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	uploader, err := transport.NewUploader(config.TransportCfg)
	if err != nil {
		return nil, err
	}

	fileWatcher := &fileWatcher{
		config:   config,
		watcher:  watcher,
		uploader: uploader,
		pool:     newWorkerPool(uploader),
	}

	for _, v := range append(config.Files, config.Folders...) {
		err := watcher.Add(v)
		if err != nil {
			return nil, err
		}
	}

	return fileWatcher, nil
}

func (fWatcher *fileWatcher) Close() error {
	var errs error
	close(fWatcher.pool.tasks)

	if err := fWatcher.watcher.Close(); err != nil {
		errors.Join(errs, fmt.Errorf("error closing file event watcher: %s", err.Error()))
	}
	if err := fWatcher.uploader.Close(); err != nil {
		errors.Join(errs, fmt.Errorf("error closing uploader %s", err.Error()))
	}
	return errs
}

func (fWatcher *fileWatcher) reload() error {
	return nil
}

func (fw *fileWatcher) watchEvents() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			path := event.Name
			if event.Has(fsnotify.Write) {
				slog.Info("new event with path", path, event.Op.String())
				fw.pool.tasks <- path
			}
		case eventErr, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			if eventErr != nil {
				slog.Error("error during file watch", "err", eventErr)
			}
		}
	}
}

func newWorkerPool(uploader transport.Uploader) *workerPool {
	const workerNum = 50
	flightMap := map[string]time.Time{}
	mutex := &sync.Mutex{}

	tasks := make(chan string)
	for range workerNum {
		go worker(tasks, uploader, mutex, flightMap)
	}
	return &workerPool{
		tasks: tasks,
	}
}

func worker(tasks <-chan string, uploader transport.Uploader, mutex *sync.Mutex, flightMap map[string]time.Time) {
	for path := range tasks {
		var f *os.File
		var processor fileProcessor
		var err error

		mutex.Lock()
		timestamp, ok := flightMap[path]

		if !ok {
			flightMap[path] = time.Now()
		} else {
			now := time.Now()
			difference := now.Sub(timestamp)
			slog.Info(difference.String())
			operand := time.Second * 5
			if difference <= operand {
				slog.Info("skip")
				mutex.Unlock()
				continue
			} else {
				flightMap[path] = now
			}
		}

		mutex.Unlock()

		processor, err = processFile(path)

		if err != nil {
			slog.Error("error processing file before upload", "error", err)
			goto END
		}

		f, err = processor.getFile()
		if err != nil {
			slog.Error("error getting result file from processor", "error", err)
			goto END
		}
		slog.Info("successfully retrieved file", "name", f.Name())

		if err = uploader.Upload(f); err != nil {
			slog.Error("error uploading files", "err", err)
		}

	END:
		err = processor.dispose()
		if err != nil {
			slog.Error("error disposing processor data", "err", err)
		}

	}
}

func executeCommand(path string, stdout *os.File, binaryName string, args ...string) error {
	cmd := exec.Command(binaryName, args...)
	cmd.Dir = filepath.Dir(path)
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type fileExtracter struct {
	file *os.File
}

func (fxtrcter *fileExtracter) dispose() error {
	if fxtrcter.file == nil {
		return nil
	}
	if err := fxtrcter.file.Close(); err != nil {
		slog.Error("error closing temp file", slog.Any("error", err.Error()))
		return err
	}
	if err := os.Remove(fxtrcter.file.Name()); err != nil {
		slog.Error("error removing file", slog.Any("error", err.Error()))
		return err
	}

	slog.Info("removed temp file ", "name", fxtrcter.file.Name())
	return nil
}

func (fxtrcter *fileExtracter) getFile() (*os.File, error) {
	if fxtrcter.file == nil {
		return nil, fmt.Errorf("file extractor file is nil")
	}
	fxtrcter.file.Seek(0, 0)
	return fxtrcter.file, nil
}

func processFile(path string) (fileProcessor, error) {
	processors := []fileProcessor{
		&gitFileProcessor{}, &defaultFileProcessor{},
	}
	var processErrors error
	for _, processor := range processors {

		if err := processor.initialize(path); err != nil {
			return nil, err
		}

		if err := processor.process(path); err != nil {
			processErrors = errors.Join(processErrors, err)
			continue
		} else {
			return processor, nil
		}

	}

	return nil, processErrors
}

type fileProcessor interface {
	process(path string) error
	initialize(path string) error
	getFile() (*os.File, error)
	dispose() error
}

type gitFileProcessor struct {
	fileExtracter
}

func (processor *gitFileProcessor) process(path string) error {
	err := executeCommand(path, processor.file, "git", "diff")
	if err != nil {
		slog.Error("couldn't create git patch from recent changes, error: ", err.Error(), slog.Any("error", err.Error()))
		return err
	}
	return nil
}

type defaultFileProcessor struct {
	fileExtracter
}

func (processor *defaultFileProcessor) process(path string) error {
	return nil
}

func (extracter *fileExtracter) initialize(path string) error {
	filename := filepath.Base(path)

	file, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("%s-*", filename))
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	extracter.file = file
	return nil
}

func getConfig() (*AppConfig, error) {
	var jsonData []byte
	path := filepath.Join(ConfigPath, configFileName)
	jsonFile, err := getConfigJSONFile(path)
	if err != nil {
		return nil, err
	}

	if jsonData, err = io.ReadAll(jsonFile); err != nil {
		return nil, err
	}

	config := &AppConfig{
		jsonFile: jsonFile,
	}

	var raw struct {
		Transport *transport.TransportConfigBase `json:"transport"`
	}

	raw.Transport = &transport.TransportConfigBase{}

	err = json.Unmarshal(jsonData, &raw)
	if err != nil {
		return nil, err
	}

	transportCfg, err := transport.GetConfigContainer(string(raw.Transport.GetType()))
	if err != nil {
		return nil, err
	}

	config.TransportCfg = transportCfg

	if err := json.Unmarshal(jsonData, config); err != nil {
		return nil, err
	}

	return config, nil
}

func Start() (*fileWatcher, error) {
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	fileWatcher, err := newfileWatcher(config)
	if err != nil {
		return nil, err
	}

	go fileWatcher.watchEvents()

	return fileWatcher, nil
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
	_, err = file.Seek(0, 0)
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
