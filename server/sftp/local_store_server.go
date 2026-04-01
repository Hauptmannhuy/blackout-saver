package sftpserver

import (
	"blackout-saver/utils"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const serverConfigName = "server_conf.json"
const configPath = "./config"

type sftpServerConfig struct {
	SSHserverKeyPrivateName string `json:"SSH_SERVER_PRIVATE_KEY_NAME" prompt:"specify	SSH server private key name"`
	SSHclientPublicKeyName  string `json:"SSH_CLIENT_PUBLIC_KEY_NAME" prompt:"specify	SSH client public key name"`
}

func ConfigureServer() {
	cfg := &sftpServerConfig{}
	file, err := os.OpenFile(filepath.Join(configPath, serverConfigName), os.O_RDWR|os.O_CREATE, 0666)

	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
		log.Println("successfully configured server config")
	}()

	if err != nil {
		panic(err)
	}

	_, err = file.Write([]byte{0})
	if err != nil {
		panic(err)
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		panic(err)
	}
	err = utils.ConfigPromptInitialize(cfg)
	if err != nil {
		panic(err)
	}

	cfgData, err := json.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	_, err = file.Write(cfgData)
	if err != nil {
		panic(err)
	}

}

func getConfig() (*sftpServerConfig, error) {
	cfg := &sftpServerConfig{}
	file, err := os.Open(filepath.Join(configPath, serverConfigName))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if decoder == nil {
		return nil, errors.New("couldn't initialize decoder")
	}

	for decoder.More() {
		err := decoder.Decode(cfg)
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func ListenSSH() {

	config, err := getConfig()
	if err != nil {
		panic(err)
	}

	clientPubBytes, err := utils.ReadSSHFile(config.SSHclientPublicKeyName)
	if err != nil {
		panic(fmt.Sprintf("reading client public key: %s", err.Error()))
	}

	clientKey, _, _, _, err := ssh.ParseAuthorizedKey(clientPubBytes)
	if err != nil {
		panic(fmt.Sprintf("parsing client public key: %s", err.Error()))
	}

	serverPrivBytes, err := utils.ReadSSHFile(config.SSHserverKeyPrivateName)
	if err != nil {
		panic(fmt.Sprintf("reading server private key: %s", err.Error()))
	}

	serverKey, err := ssh.ParsePrivateKey(serverPrivBytes)
	if err != nil {
		panic(fmt.Sprintf("parsing server private key: %s", err.Error()))
	}

	listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("0.0.0.0:5001")))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("initialized tcp listener, waiting for incoming requests")
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Fatal(err)
		}
		algorithms := ssh.SupportedAlgorithms()
		serverConfig := &ssh.ServerConfig{
			PublicKeyAuthAlgorithms: algorithms.PublicKeyAuths,
			PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {

				if err != nil {
					return nil, err
				}

				if !bytes.Equal(key.Marshal(), clientKey.Marshal()) {
					return nil, fmt.Errorf("unathorized")
				}
				return nil, nil
			},
		}

		if err != nil {
			log.Fatal(err)
		}
		serverConfig.AddHostKey(serverKey)
		_, rootChannel, rootReqCh, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("ssh server initialized")

		go func() {
			for {
				select {
				case req := <-rootReqCh:
					fmt.Println(req.Type, string(req.Payload), req.WantReply, req.Type == "establish sftp")
				case ch := <-rootChannel:
					fmt.Println("new channel", ch.ChannelType(), "extracted data", string(ch.ExtraData()))
					newCh, newReqCh, err := ch.Accept()
					if err != nil {
						log.Fatal(err)
					}
					go listenForSFTPRequest(newCh, newReqCh)
				}
			}
		}()

	}
}

func listenForSFTPRequest(ch ssh.Channel, req <-chan *ssh.Request) {
	sftpChan := make(chan ssh.Channel)

	go func() {
		fmt.Println("waiting for sftp chan")
		conn := <-sftpChan
		server, err := sftp.NewServer(conn)
		fmt.Println("created sftp server and listening")
		if err != nil {
			panic(err)
		}
		if err := server.Serve(); err != nil {
			panic(err)
		}
	}()

	for inReq := range req {
		if inReq == nil {
			continue
		}

		if strings.Contains(string(inReq.Payload), "sftp") {
			fmt.Println("sftp request received")
			var err error

			err = inReq.Reply(true, []byte("ok"))
			if err != nil {
				panic(err)
			}
			sftpChan <- ch
		} else {
			fmt.Println("received not interesting request")
			err := ch.Close()
			if err != nil {
				panic(err)
			}
			return
		}
	}
}
