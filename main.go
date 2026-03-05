package main

import (
	"blackout-saver/internal"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	args := os.Args
	if len(args[1:]) == 0 {
		panic("no args are specified")
	}

	startCmd := flag.CommandLine.Bool("start", true, "starts service")
	flag.Parse()
	if startCmd != nil {
		internal.Start()
	}

	term := make(chan os.Signal, 1)

	signal.Notify(term, syscall.SIGTERM|syscall.SIGINT)
	<-term
	fmt.Println("exiting")
}
