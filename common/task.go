package common

import "fmt"

const INVALID_LOC = -1

// schema for location (task or vehicle)
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// schema for mobile task
type Task struct {
	AppID       int      `json:"app_id"`
	Location    Location `json:"location"`
	Destination Location `json:"destination"`
	RequestTime int      `json:"request_time"`
}

func (t Task) String() string {
	return fmt.Sprintf(
		"(%0.6f, %0.6f, %d, %d)",
		t.Location.Latitude,
		t.Location.Longitude,
		t.AppID,
		t.RequestTime,
	)
}
