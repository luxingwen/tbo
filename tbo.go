package tbo

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

var global *params

type params struct {
	*flag.FlagSet `json:"-"`

	tboFile  string
	lexCache int

	debug bool
}

func Main() {
	global = &params{}
	global.FlagSet = flag.NewFlagSet("TBO Global Params", flag.ContinueOnError)

	global.StringVar(&global.tboFile, "tbo", "", "tbo file")
	global.IntVar(&global.lexCache, "lex-cache", 30, "tbo lex cache size")

	global.BoolVar(&global.debug, "debug", false, "open debug log")

	var err error

	err = global.Parse(os.Args[1:])
	if err != nil {
		os.Exit(0)
	}

	if global.debug {
		log.SetLevel(log.DebugLevel)
	}
	data, err := ioutil.ReadFile(filepath.Clean(global.tboFile))
	if err != nil {
		fmt.Printf("Read TBO File Error:\n\t%s\n\n", err.Error())
		global.Usage()
		os.Exit(0)
	}

	parser, err := parse(string(data))
	if err != nil {
		fmt.Printf("Parse TBO File Error:\n\t%s\n\n", err.Error())
		global.Usage()
		os.Exit(0)
	}

	err = parser.check()
	if err != nil {
		fmt.Printf("Check TBO Data Error:\n\t%s\n\n", err.Error())
		global.Usage()
		os.Exit(0)
	}

	err = print(parser.prints)
	if err != nil {
		fmt.Printf("Print TBO Data Error:\n\t%s\n\n", err.Error())
		global.Usage()
		os.Exit(0)
	}

	log.Printf("\nSucc!\n")

}

type tboError string

func (e tboError) Error() string {
	return string(e)
}

func throw(format string, v ...interface{}) {
	log.Debugf(format, v...)
	panic(tboError(fmt.Sprintf(format, v...)))
}
