package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"blackout-saver/internal"
)

type cliCommand string

const (
	start     cliCommand = "start"
	addDir    cliCommand = "add_dir"
	transport cliCommand = "transport"
)

func main() {
	args := os.Args
	if len(args[1:]) == 0 {
		panic("no args are specified")
	}

	cmd := cliCommand(args[1])
	args = args[1:]
	flag.CommandLine.Bool("start", false, "starts service")
	flag.CommandLine.String("add_dir", "", "makes watch over given directories")
	flag.CommandLine.String("transport", "", "choose transport method to send files")

	flag.Parse()

	commandHandlers := map[cliCommand]func(){
		start: func() {
			if err := internal.Start(); err != nil {
				log.Fatal(err)
			}
			term := make(chan os.Signal, 1)

			signal.Notify(term, syscall.SIGTERM|syscall.SIGINT)
			<-term
			fmt.Println("exiting")

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
		transport: func() {
			if len(args) <= 1 {
				panic("you have to provide a transport type ('sftp')")
			}
			internal.SetTransport(args[1])
		},
	}
	fmt.Println(cmd)
	fn, ok := commandHandlers[cmd]
	if ok {
		fn()
	} else {
		log.Fatalf("unknown command %s", cmd)
	}
}
