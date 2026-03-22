package sftpserver

import (
	"blackout-saver/utils"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func getEnv(name string) (string, error) {
	val := os.Getenv(name)
	if len(val) == 0 {
		return "", fmt.Errorf("%s should not be empty", name)
	}
	return val, nil
}

type SFTPserverConfig struct {
	clientPubKey []ssh.PublicKey
	serverKey    ssh.Signer
}

func getConfig() (*SFTPserverConfig, error) {
	clientPubName, err := getEnv("SFTP_SERVER_PRIVATE_KEY_NAME")
	if err != nil {
		return nil, err
	}
	serverPrivName, err := getEnv("SFTP_CLIENT_PUBLIC_KEY_NAME")
	if err != nil {
		return nil, err
	}

	clientPubBytes, err := utils.ReadSSHFile(clientPubName)
	if err != nil {
		return nil, fmt.Errorf("reading client public key: %w", err)
	}

	clientKey, _, _, _, err := ssh.ParseAuthorizedKey(clientPubBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing client public key: %w", err)
	}

	serverPrivBytes, err := utils.ReadSSHFile(serverPrivName)
	if err != nil {
		return nil, fmt.Errorf("reading server private key: %w", err)
	}

	serverKey, err := ssh.ParsePrivateKey(serverPrivBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing server private key: %w", err)
	}

	return &SFTPserverConfig{
		clientPubKey: []ssh.PublicKey{clientKey},
		serverKey:    serverKey,
	}, nil
}

func ListenSSH() {

	config, err := getConfig()
	if err != nil {
		panic(err)
	}
	listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(netip.MustParseAddrPort("0.0.0.0:5001")))
	if err != nil {
		log.Fatal(err)
	}
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

				if !bytes.Equal(key.Marshal(), config.clientPubKey[0].Marshal()) {
					return nil, fmt.Errorf("unathorized")
				}
				return nil, nil
			},
			// },
		}
		if err != nil {
			log.Fatal(err)
		}
		serverConfig.AddHostKey(config.serverKey)
		_, channel, reqCh, err := ssh.NewServerConn(conn, serverConfig)
		if err != nil {
			log.Fatal(err)
		}
		go func() {

			for {
				select {
				case req := <-reqCh:
					fmt.Println(req.Type, string(req.Payload), req.WantReply, req.Type == "establish sftp")
				case ch := <-channel:
					fmt.Println("new channel", ch.ChannelType(), "extracted data", string(ch.ExtraData()))
					newCh, newReqCh, err := ch.Accept()
					if err != nil {
						log.Fatal(err)
					}
					go newChannelSession(newCh, newReqCh)
				}
			}
		}()

	}
}

func newChannelSession(ch ssh.Channel, req <-chan *ssh.Request) {
	var buf []byte
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

	for {
		select {
		case req := <-req:
			if req == nil {
				continue
			}

			if strings.Contains(string(req.Payload), "sftp") {
				fmt.Println("sftp request received")
				var err error

				err = req.Reply(true, []byte("ok"))
				if err != nil {
					panic(err)
				}
				sftpChan <- ch
			}

		default:
			n, err := ch.Read(buf)
			if err != nil {
				fmt.Println(err)
			}
			if n > 0 {
				fmt.Println(string(buf[:n]))
			}
		}
	}
}
