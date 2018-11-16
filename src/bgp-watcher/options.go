package main

// ##### Structs ##############################################################

type Options struct {
	Verbose bool `short:"v" long:"verbose" description:"Show verbose debug information"`
	Reparse bool `short:"r" long:"reparse" description:"Performs history re-parse"`
}
