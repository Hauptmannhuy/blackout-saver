package transport

import (
	"blackout-saver/utils"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Uploader interface {
	Upload(data *os.File) error
	Close() error
	Connect() error
}

type transportType string

const (
	SFTP transportType = "sftp"
)

type TransportConfigBase struct {
	TransportType transportType `json:"transport_type"`
}

type SFTPUploader struct {
	cfg        *SFTPconfig
	sftpClient *sftp.Client
	sshClient  *ssh.Client
}

type SFTPconfig struct {
	TransportConfigBase
	RemoteServerPub string `json:"remote_server_pub" prompt:"Specify path for remote server public key"`
	ClientPrivate   string `json:"client_private" prompt:"Specify path for client private key"`
}

type TransportConfig interface {
	GetType() transportType
	SetTransportType(name transportType)
}

func (t TransportConfigBase) GetType() transportType {
	return t.TransportType
}

func (t *TransportConfigBase) SetTransportType(name transportType) {
	t.TransportType = name
}

func (uploader *SFTPUploader) Connect() error {
	cfg := uploader.cfg
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	clientKey, err := utils.ReadSSHFile(cfg.ClientPrivate)
	if err != nil {
		return fmt.Errorf("client ssh key not found: %w, generate it with ssh-keygen, example ssh-keygen -t ed25519, path: .../.ssh/id_ed25519_client_blackout_sftp", err)
	}

	private, err := ssh.ParsePrivateKey(clientKey)
	if err != nil {
		return err
	}

	addr := "0.0.0.0:5001"
	slog.Info("trying to connect to ", "ip", addr)
	sshClient, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(private),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			serverKey, err := utils.ReadSSHFile(cfg.RemoteServerPub)
			if err != nil {
				return err
			}
			k, _, _, _, err := ssh.ParseAuthorizedKey(serverKey)
			if err != nil {
				return err
			}
			if bytes.Equal(key.Marshal(), k.Marshal()) {
				return nil
			}

			return fmt.Errorf("unauthorized")
		},
	})
	if err != nil {
		return err
	}

	slog.Info("trying to establish sftp...")
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return err
	}
	slog.Info("sftp connection established")
	uploader.sftpClient = client
	uploader.sshClient = sshClient
	return nil
}

func GetConfigContainer(t string) (TransportConfig, error) {
	switch transportType(t) {
	case SFTP:
		return &SFTPconfig{}, nil
	default:
		return nil, fmt.Errorf("given transport type %s is not implemented", t)
	}
}

func NewTransportConfig() (TransportConfig, error) {
	fmt.Println("You can specify only either of ", []transportType{SFTP})
	input, err := utils.GetInput()
	if err != nil {
		return nil, err
	}

	t := transportType(input)
	var transportCfg TransportConfig
	if t == "" {
		return nil, fmt.Errorf("unknown transport type")
	}
	transportCfg, err = GetConfigContainer(input)
	if err != nil {
		return nil, err
	}

	if err := utils.ConfigPromptInitialize(transportCfg); err != nil {
		return nil, err
	}
	transportCfg.SetTransportType(t)
	return transportCfg, nil
}

func NewUploader(config TransportConfig) (Uploader, error) {
	var uploader Uploader
	var err error
	switch cfg := config.(type) {
	case *SFTPconfig:
		uploader = &SFTPUploader{
			cfg: cfg,
		}
	}

	retries := 15

	for i := 1; i <= retries; i++ {
		err = uploader.Connect()
		if err == nil {
			slog.Info("connected to remote storage")
			break
		}

		var netErr net.Error
		if !errors.As(err, &netErr) {
			slog.Error("couldn't connect to remote storage via client error, exiting", "error", err)
			break
		}

		slog.Warn("couldn't connect to remote storage server",
			"error", err,
			"attempt", i,
			"max_retries", retries,
		)

		if i < retries {
			time.Sleep(2 * time.Second)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect after %d retries: %w", retries, err)
	}
	return uploader, err

}

func (sftpUploader SFTPUploader) Upload(file *os.File) error {
	sftpClient := sftpUploader.sftpClient
	remotePath := filepath.Join("storage", file.Name())
	if err := sftpClient.MkdirAll(filepath.Dir(remotePath)); err != nil {
		slog.Info("error creating dir", err.Error(), remotePath)
	}

	sftpFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("error creating static storage %s", err.Error())

	}

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("couldn't read file %s, error: %s", file.Name(), err.Error())
	}

	_, err = sftpFile.Write(data)
	if err != nil {
		return fmt.Errorf("error writing to received sftp file %w", err)

	}

	if err := sftpFile.Close(); err != nil {
		return fmt.Errorf("error closing sftp file %w", err)
	}

	return nil
}

func (uploader *SFTPUploader) Close() error {
	var err error
	if sshErr := uploader.sshClient.Close(); sshErr != nil {
		errors.Join(err, sshErr)
	}
	if sftpErr := uploader.sftpClient.Close(); sftpErr != nil {
		return errors.Join(err, sftpErr)
	}
	return err
}
