package tbo

import (
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/tealeg/xlsx"
)

type tboExecl struct {
	name   string
	paths  []string
	sheets sync.Map
}

func (e *tboExecl) loadFiles(wait *sync.WaitGroup, errc chan error) {

	var inFiles []string

	for _, path := range e.paths {
		filepaths, err := filepath.Glob(path)
		if err != nil {
			errc <- err
			return
		}
		inFiles = append(inFiles, filepaths...)
	}

	for _, infilepath := range inFiles {
		wait.Add(1)
		go func(filePath string) {

			defer wait.Done()

			log.Debugf("Execl File Path:%s", filePath)

			file, err := xlsx.OpenFile(filePath)

			if err != nil {
				errc <- err
				return
			}

			for _, sheet := range file.Sheets {
				name := strings.Split(sheet.Name, "|")[0]
				log.Debugf("Execl:%s Sheet:%s ", e.name, name)
				e.sheets.Store(name, sheet)
			}

		}(infilepath)

	}
}

func (e *tboExecl) sheet(name string) (sheet *xlsx.Sheet, ok bool) {
	isheet, iok := e.sheets.Load(name)
	if !iok {
		return
	}
	sheet, ok = isheet.(*xlsx.Sheet)
	return
}
