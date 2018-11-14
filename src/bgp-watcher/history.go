package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx"
)

// ##### Structs ##############################################################

//
type History struct {
	Months    int
	Processes int
}

// ##### Methods ##############################################################

func NewHistory(months int, processes int) (*History, error) {
	return &History{
		Months:    months,
		Processes: processes,
	}, nil
}

// Downloads and loads/parses the BGP update files
func (h *History) Update() {

	// Get a constant value for NOW
	ts := time.Now()

	h.download(ts)
	h.parse(ts)
}

func (h *History) download(ts time.Time) {

	var err error
	var year int
	var month int

	for i := h.Months - 1; i >= 0; i-- {

		year = int(ts.AddDate(0, -i, 0).Year())
		month = int(ts.AddDate(0, -i, 0).Month())

		err = checkDirectories(year, month)
		if err != nil {
			fmt.Printf("Error validating directory stores: %v", err)
			continue
		}

		h.downloadUpdateFiles(year, month)
	}
}

// Downloads RIPE page containing BGP update files, using a specific year/month
// index. Parses the page for update files, checks if the file has already been
// downloaded and the file header checked (GZIP)
func (h *History) downloadUpdateFiles(year int, month int) {

	files, err := getUpdateFiles(year, month)
	if err != nil {
		return
	}

	// Now perform the actual downloading concurrently
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, h.Processes)
	for _, file := range files {
		wg.Add(1)

		go func(year int, month int, fileName string) {
			defer wg.Done()

			semaphore <- struct{}{} // Lock
			defer func() {
				<-semaphore // Unlock
			}()

			fmt.Printf("Uncached update file: %s\n", fileName)
			err = downloadUpdateFile(year, month, fileName)
			if err != nil {
				fmt.Printf("Error downloading update file (%s): %v\n", fileName, err)
			} else {

			}

		}(year, month, file)
	}
	wg.Wait()
}

//
func (h *History) parse(ts time.Time) {

	fmt.Println("START")
	fmt.Println(time.Now())

	var year int
	var month int
	asns := make(map[uint32]map[string]uint64)

	mrtParser := new(MrtParser)
	for i := h.Months - 1; i >= 0; i-- {

		year = int(ts.AddDate(0, -i, 0).Year())
		month = int(ts.AddDate(0, -i, 0).Month())

		files, err := ioutil.ReadDir(fmt.Sprintf("./cache/%v/%v", year, month))
		if err != nil {
			log.Fatal(err)
		}

		var wg sync.WaitGroup
		semaphore := make(chan struct{}, h.Processes)
		for _, file := range files {
			wg.Add(1)

			go func(asns map[uint32]map[string]uint64, year int, month int, filePath string) {
				defer wg.Done()

				semaphore <- struct{}{} // Lock
				defer func() {
					<-semaphore // Unlock
				}()

				asns, err = mrtParser.ParseAndCollect(asns, fmt.Sprintf("./cache/%v/%v/%s", year, month, filePath))
				if err != nil {
					if strings.Contains(err.Error(), "gzip: invalid header") == true {
						err = os.Remove(fmt.Sprintf("./cache/%v/%v/%s", year, month, filePath))
						if err != nil {
							fmt.Println("Error deleting malformed BGP file (%s): %v\n", fmt.Sprintf("./cache/%v/%v/%s", year, month, filePath), err)
						}
					} else {
						fmt.Println("Error parsing BGP file (%s): %v\n", filePath, err)
					}
				}
			}(asns, year, month, file.Name())
		}
		wg.Wait()
	}

	storeUpdates(asns)

	fmt.Println("FINISH")
	fmt.Println(time.Now())
}

//
func storeUpdates(data map[uint32]map[string]uint64) {

	var rows [][]interface{}

	for peer, a := range data {
		for route, count := range a {
			rows = append(rows, []interface{}{peer, route, count})
		}
	}

	_, err := db.CopyFrom(
		pgx.Identifier{"routes"},
		[]string{"peer_as", "route", "count"},
		pgx.CopyFromRows(rows))

	if err != nil {
		fmt.Printf("Error inserting historic data: %v\n", err)
		return
	}
}
