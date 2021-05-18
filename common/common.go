package common

import (
	"encoding/csv"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"runtime"
)

// marshal data structure to JSON
func ToJSON(x interface{}) []byte {
	bytes, err := json.MarshalIndent(x, "", "\t")
	if err != nil {
		log.Fatalf("[common] error marshaling %T to JSON: %v", x, err)
	}
	return bytes
}

// read JSON from file, unmarshal into data structure
func FromFile(path string, x interface{}) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("[common] error opening file %s: %v", path, err)
	}
	defer file.Close()

	// read file as byte array
	bytes, _ := ioutil.ReadAll(file)
	if err := json.Unmarshal(bytes, x); err != nil {
		log.Fatalf(
			"[common] error unmarshaling json to output struct %T: %v (%s)",
			x,
			err,
			path,
		)
	}
}

// marshal data structure to JSON, write to file
func ToFile(path string, x interface{}) {
	bytes := ToJSON(x)

	// write byte array to file
	if err := ioutil.WriteFile(path, bytes, 0644); err != nil {
		log.Fatalf("[common] error writing struct %T to file: %v", x, err)
	}
}

// get directory of package file currently being executed
func GetDir() string {
	_, filename, _, _ := runtime.Caller(1)
	return path.Dir(filename)
}

// get min/max of []float64 slice
func GetMinMax(x []float64) (float64, float64) {
	min := 1000.0
	max := 0.0
	for _, a := range x {
		if a > max {
			max = a
		}
		if a < min {
			min = a
		}
	}
	return min, max
}

// create CSV writer
func CreateCSVWriter(path string) *csv.Writer {
	file, err := os.Create(path)
	if err != nil {
		log.Fatalf("[common] error creating CSV writer: %v", err)
	}

	return csv.NewWriter(file)
}
