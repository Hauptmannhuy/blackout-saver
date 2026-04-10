package sftpserver

import (
	"blackout-saver/utils"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
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

type sftpServer struct {
	listener       *net.TCPListener
	sshConnections []*ssh.ServerConn
	sftpConns      []*sftp.Server
}

type sftpServerKeys struct {
	clientPubKey    []byte
	clientKey       ssh.PublicKey
	serverPrivBytes []byte
	serverKey       ssh.Signer
}

func ConfigureServer() error {
	cfg := &sftpServerConfig{}
	file, err := os.OpenFile(filepath.Join(configPath, serverConfigName), os.O_RDWR|os.O_CREATE, 0666)

	defer func() error {
		if err := file.Close(); err != nil {
			return err
		}
		slog.Info("successfully configured server config")
		return nil
	}()

	if err != nil {
		return err
	}

	_, err = file.Write([]byte{0})
	if err != nil {
		return err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}
	err = utils.ConfigPromptInitialize(cfg)
	if err != nil {
		return err
	}

	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = file.Write(cfgData)
	if err != nil {
		return err
	}
	return nil
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

func NewSFTPServer() (*sftpServer, error) {
	listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("0.0.0.0:5001")))
	if err != nil {
		log.Fatal(err)
	}

	slog.Info("initialized tcp listener, waiting for incoming requests")
	return &sftpServer{
		listener: listener,
	}, nil

}

func (server *sftpServer) getKeys() (*sftpServerKeys, error) {
	config, err := getConfig()
	if err != nil {
		panic(err)
	}

	clientPubBytes, err := utils.ReadSSHFile(config.SSHclientPublicKeyName)
	if err != nil {
		return nil, fmt.Errorf("reading client public key: %s", err.Error())
	}

	clientKey, _, _, _, err := ssh.ParseAuthorizedKey(clientPubBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing client public key: %s", err.Error())
	}

	serverPrivBytes, err := utils.ReadSSHFile(config.SSHserverKeyPrivateName)
	if err != nil {
		return nil, fmt.Errorf("reading server private key: %s", err.Error())
	}

	serverKey, err := ssh.ParsePrivateKey(serverPrivBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing server private key: %s", err.Error())
	}
	return &sftpServerKeys{
		clientPubKey:    clientPubBytes,
		clientKey:       clientKey,
		serverPrivBytes: serverPrivBytes,
		serverKey:       serverKey,
	}, nil
}

func closeIO[T io.Closer](errs error, items ...T) {
	for _, c := range items {
		if err := c.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
}

func (server *sftpServer) Close() error {
	var errs error

	closeIO(errs, server.listener)
	closeIO(errs, server.sftpConns...)
	closeIO(errs, server.sshConnections...)

	return errs
}

func (server *sftpServer) Listen() {
	keys, err := server.getKeys()
	if err != nil {
		panic(err)
	}

	for {
		conn, err := server.listener.AcceptTCP()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				slog.Info("closing tcp listener")
				return
			}
		}
		algorithms := ssh.SupportedAlgorithms()
		serverConfig := &ssh.ServerConfig{
			PublicKeyAuthAlgorithms: algorithms.PublicKeyAuths,
			PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {

				if err != nil {
					return nil, err
				}

				if !bytes.Equal(key.Marshal(), keys.clientKey.Marshal()) {
					return nil, fmt.Errorf("unathorized")
				}
				return nil, nil
			},
		}

		if err != nil {
			log.Fatal(err)
		}
		serverConfig.AddHostKey(keys.serverKey)
		sshServer, rootChannel, rootReqCh, err := ssh.NewServerConn(conn, serverConfig)
		server.sshConnections = append(server.sshConnections, sshServer)

		if err != nil {
			log.Fatal(err)
		}
		slog.Info("ssh server initialized")

		go func() {
			for {
				select {
				case req := <-rootReqCh:
					if req == nil {
						continue
					}
				case ch := <-rootChannel:
					if ch == nil {
						continue
					}
					slog.Info("new channel", "new channel", ch.ChannelType(), "extracted data", string(ch.ExtraData()))
					newCh, newReqCh, err := ch.Accept()
					if err != nil {
						log.Fatal(err)
					}
					go server.handleSSHmsg(newCh, newReqCh)
				}
			}
		}()

	}
}

func (server *sftpServer) handleSSHmsg(ch ssh.Channel, req <-chan *ssh.Request) {
	for inReq := range req {
		if inReq == nil {
			continue
		}
		if strings.Contains(string(inReq.Payload), "sftp") {
			slog.Info("sftp request received")
			var err error

			err = inReq.Reply(true, []byte("ok"))
			if err != nil {
				panic(err)
			}
			go server.serveSFTP(ch)
		} else {
			fmt.Printf("received ssh message %s\n", string(inReq.Payload))
			err := ch.Close()
			if err != nil {
				panic(err)
			}
			return
		}
	}
	if err := ch.Close(); err != nil {
		if errors.Is(err, io.EOF) {
			slog.Info(err.Error())
			return
		}
		slog.Info("error in ssh channel", slog.Any("error:", err))
	}

}

func (server *sftpServer) serveSFTP(conn io.ReadWriteCloser) {

	newConn, err := sftp.NewServer(conn, sftp.WithDebug(os.Stderr))
	if err != nil {
		panic(err)
	}
	slog.Info("created sftp server and listening")

	server.sftpConns = append(server.sftpConns, newConn)
	if err := newConn.Serve(); err != nil {
		panic(err)
	}
}
