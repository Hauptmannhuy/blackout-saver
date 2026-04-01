package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"blackout-saver/internal"
	sftpserver "blackout-saver/server/sftp"
	"blackout-saver/transport"
)

type cliCommand string

const (
	start              cliCommand = "start"
	addDir             cliCommand = "add_dir"
	setTransport       cliCommand = "transport"
	server             cliCommand = "server"
	configureSSHServer cliCommand = "config_server_ssh"
	//debug
	connect cliCommand = "connect"
)

func runForever(fn func()) {
	go fn()
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM|syscall.SIGINT)
	<-term
	fmt.Println("exiting")
}

func main() {
	args := os.Args
	if len(args[1:]) == 0 {
		panic("no args are specified")
	}

	cmd := cliCommand(args[1])
	args = args[1:]
	flag.CommandLine.Bool(string(start), false, "starts service")
	flag.CommandLine.String(string(addDir), "", "makes watch over given directories")
	flag.CommandLine.Bool(string(setTransport), false, "choose transport method to send files")
	flag.CommandLine.String(string(server), "", "connects to the local ssh server")
	flag.CommandLine.String(string(connect), "", "starts local ssh server")
	flag.CommandLine.Bool(string(configureSSHServer), false, "configures ssh server")
	flag.Parse()

	commandHandlers := map[cliCommand]func(){
		start: func() {
			runForever(func() {
				internal.Start()
			})
		},
		addDir: func() {
			if len(args) <= 1 {
				panic("you have to provide a file path")
			}
			filePath := args[1]
			if err := internal.AddDirToWatch(filePath); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("successfully added %s", filePath)

		},
		setTransport: func() {
			internal.SetTransport()
		},

		server: func() {
			runForever(func() {
				sftpserver.ListenSSH()
			})
		},

		configureSSHServer: func() {
			sftpserver.ConfigureServer()
		},
		//for debugging
		connect: func() {
			runForever(func() {
				transport.NewUploader(&transport.SFTPconfig{
					RemoteServerPub: "server_sftp_blackout_key.pub",
					ClientPrivate:   "id_ed25519_client_blackout_sftp",
				})
			})
		},
	}
	fn, ok := commandHandlers[cmd]
	if ok {
		fn()
	} else {
		log.Fatalf("unknown command %s", cmd)
	}
}
