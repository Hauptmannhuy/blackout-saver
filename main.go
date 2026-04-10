package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"blackout-saver/internal"
	sftpserver "blackout-saver/server/sftp"
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

	config := slog.HandlerOptions{
		AddSource: true,
	}
	handler := slog.NewTextHandler(os.Stderr, &config)

	slog.SetDefault(slog.New(handler))

	commandHandlers := map[cliCommand]func(){
		start: func() {
			fWatcher, err := internal.Start()
			if err != nil {
				log.Fatal("error starting app", err)
			}
			term := make(chan os.Signal, 1)
			signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)
			slog.Info("application succesfully started")
			sig := <-term
			fmt.Println("received", "signal", sig.String())

			if err := fWatcher.Close(); err != nil {
				fmt.Println(err)
			}
			fmt.Println("app gracefully shutted down")
		},
		addDir: func() {
			if len(args) <= 1 {
				panic("you have to provide a file path")
			}
			filePath := args[1]
			if err := internal.AddDirToWatch(filePath); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("successfully added %s\n", filePath)

		},
		setTransport: func() {
			internal.SetTransport()
		},

		server: func() {
			server, err := sftpserver.NewSFTPServer()
			if err != nil {
				panic(err)
			}
			go server.Listen()
			term := make(chan os.Signal, 1)
			signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)
			<-term
			if err := server.Close(); err != nil {
				fmt.Println("error during server close", err)
				return
			}
			fmt.Println("server shutted down gracefully")
		},

		configureSSHServer: func() {
			err := sftpserver.ConfigureServer()
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	fn, ok := commandHandlers[cmd]
	if ok {
		fn()
	} else {
		log.Fatalf("unknown command %s", cmd)
	}
}
