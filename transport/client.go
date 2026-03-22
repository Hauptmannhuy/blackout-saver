package transport

import (
	"blackout-saver/utils"
	"bytes"
	"fmt"
	"log"
	"net"
	"reflect"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Uploader interface {
	Upload()
}

type transportType string

const (
	SFTP transportType = "sftp"
)

type TransportConfigBase struct {
	TransportType transportType `json:"transport_type"`
}

type SFTPUploader struct {
	cfg        SFTPconfig
	sftpClient *sftp.Client
	sshClient  *ssh.Client
}

type SFTPconfig struct {
	TransportConfigBase
	RemoteServerPub string `json:"remote_server_pub" prompt:"Specify path for remote server public key"`
	ClientPrivate   string `json:"client_private" prompt:"Specify path for client private key"`
}

// func (cfg SFTPconfig) UnmarshalJSON(data []byte) error {
// 	var t struct {
// 	}
// 	return json.Unmarshal(data, &cfg)
// }

type TransportConfig interface {
	GetType() transportType
	SetTransportType(name transportType)
}

func (t TransportConfigBase) GetType() transportType {
	return t.TransportType
}

func (t TransportConfigBase) SetTransportType(name transportType) {
	t.TransportType = name
}

func (sftpUloader SFTPUploader) connectSSH(cfg SFTPconfig) error {
	clientKey, err := utils.ReadSSHFile(cfg.ClientPrivate)
	if err != nil {
		log.Fatal(fmt.Errorf("client ssh key not found: %w, generate it with ssh-keygen, example ssh-keygen -t ed25519, path: .../.ssh/id_ed25519_client_blackout_sftp", err))
	}

	private, err := ssh.ParsePrivateKey(clientKey)
	if err != nil {
		log.Fatal(err)
	}

	sshClient, err := ssh.Dial("tcp", "0.0.0.0:5001", &ssh.ClientConfig{
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
		log.Fatal(err)
	}

	time.Sleep(time.Second * 2)
	fmt.Println("trying to establish sftp...")
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		log.Fatal(err)
	}

	sftpUloader.sftpClient = client
	sftpUloader.sshClient = sshClient
	return nil
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
	switch t {
	case SFTP:
		transportCfg = &SFTPconfig{}
	default:
		return nil, fmt.Errorf("given transport type %s is not implemented", t)
	}

	refVal := reflect.ValueOf(transportCfg).Elem()
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
		fmt.Println(prompt)
		input, err := utils.GetInput(prompt)
		if err != nil {
			return nil, err
		}

		fieldVal := refVal.FieldByName(field.Name)
		if fieldVal.IsValid() != true {
			return nil, fmt.Errorf("error occured during setting fields %s field on %s struct is not valid", field.Name, refT.Name())
		}
		fieldVal.Set(reflect.ValueOf(input))
	}
	transportCfg.SetTransportType(t)
	return transportCfg, nil
}

func NewUploader(config TransportConfig) (Uploader, error) {
	var uploader Uploader
	var err error
	switch cfg := config.(type) {
	case SFTPconfig:
		uploader, err = newSFTPUploader(cfg)
	}

	return uploader, err
}

func newSFTPUploader(config SFTPconfig) (SFTPUploader, error) {
	uploader := SFTPUploader{}
	err := uploader.connectSSH(config)
	return uploader, err
}

func (sftpUploader SFTPUploader) Upload() {

}
