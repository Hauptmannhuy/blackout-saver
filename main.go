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

func main() {
	args := os.Args
	if len(args[1:]) == 0 {
		panic("no args are specified")
	}

	args = args[1:]

	startCmd := flag.CommandLine.Bool("start", false, "starts service")
	addDirCmd := flag.CommandLine.String("add_dir", "", "makes watch over given directories")

	flag.Parse()
	fmt.Println(*startCmd)
	fmt.Println(*addDirCmd)
	if startCmd != nil && *startCmd == true {
		if err := internal.Start(); err != nil {
			log.Fatal(err)
		}
		term := make(chan os.Signal, 1)

		signal.Notify(term, syscall.SIGTERM|syscall.SIGINT)
		<-term
		fmt.Println("exiting")

	}
	fmt.Println(args)
	if addDirCmd != nil && *addDirCmd != "" {
		if len(args) <= 1 {
			panic("you have to provide a file path")
		}
		filePath := args[1]
		if err := internal.AddDirToWatch(filePath); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("successfully added %s", filePath)
	}
}
