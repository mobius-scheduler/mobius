package common

type Vehicle struct {
	ID       int      `json:"id"`
	Location Location `json:"location"`
	Speed    float64  `json:"speed"`
}
