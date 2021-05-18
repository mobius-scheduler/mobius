package vrp

import (
	"fmt"
	"github.com/mobius-scheduler/mobius/common"
)

// schema for route for a single vehicle in schedule
type Route struct {
	Path          []common.TaskData `json:"path"`
	TotalInterest float64           `json:"total_interest"`
	TotalTime     int               `json:"total_time"`
	VehicleStart  common.Location   `json:"vehicle_start"`
	VehicleEnd    common.Location   `json:"vehicle_end"`
}

// schema for allocation: app ID --> allocated interest
type Allocation map[int]float64

func (a Allocation) String() string {
	out := "{"
	for id, x := range a {
		out += fmt.Sprintf("app %d: %0.1f,", id, x)
	}
	out += "}"
	return out
}

func (a Allocation) Total() float64 {
	var sum float64
	for _, x := range a {
		sum += x
	}
	return sum
}

// schema for schedule returned by VRP solver
type Schedule struct {
	Routes     []Route    `json:"routes"`
	Allocation Allocation `json:"allocation"`
	Stats      struct {
		Weights map[int]float64 `json:"weights"`
		Alpha   float64         `json:"alpha"`
		Bound   float64         `json:"bound"`
	} `json:"stats"`
}

func (s *Schedule) Trim(time int) {
	// init alloc
	alloc := make(Allocation)
	for id, _ := range s.Allocation {
		alloc[id] = 0
	}

	// trim schedule
	for i, route := range s.Routes {
		var j int
		var t common.TaskData
		if len(route.Path) == 0 {
			continue
		}
		for j, t = range route.Path {
			if t.Destination.Latitude != common.INVALID_LOC && t.Destination.Longitude != common.INVALID_LOC {
				alloc[t.AppID] += 1
			}
			if t.FulfillTime > time {
				break
			}
		}

		// finish request if en route to dropoff
		if s.Routes[i].Path[j].Destination.Latitude == 0 && j < len(s.Routes[i].Path)-1 {
			j += 1
		}

		s.Routes[i].Path = s.Routes[i].Path[:j+1]
		s.Routes[i].VehicleEnd = t.Location
		s.Allocation = alloc
	}
}

func (s Schedule) String() string {
	out := "schedule has allocation {"
	for id, a := range s.Allocation {
		out += fmt.Sprintf("app %d: %0.1f, ", id, a)
	}
	out += "}"
	return out
}

func (s Schedule) ElapsedTime() []int {
	elapsed_time := make([]int, len(s.Routes))
	for i, r := range s.Routes {
		elapsed_time[i] = r.TotalTime
	}
	return elapsed_time
}

func (s Schedule) MaxTime() int {
	et := s.ElapsedTime()
	max := 0
	for _, t := range et {
		if t > max {
			max = t
		}
	}
	return max
}

// schema for solver input
type Input struct {
	InterestMap           common.InterestFile `json:"interest_map"`
	UnweightedInterestMap common.InterestFile `json:"unweighted_interest_map"`
	Vehicles              []common.Vehicle    `json:"vehicles"`
	Budget                int                 `json:"budget"`
	Capacity              int                 `json:"capacity"`
	InitialSchedule       Schedule            `json:"initial_schedule"`
	TravelTimeMatrixPath  string              `json:"travel_time_matrix_path"`
	RTH                   []common.Location   `json:"rth"`
}

// interface to VRP solvers
type Solver interface {
	New() Solver
	Solve() Schedule
	SetInterestMap(common.InterestMap)
	GetInterestMap() common.InterestMap
	GetRTH() []common.Location
	SetInitialSchedule(Schedule)
	SetTravelTimeMatrixPath(string)
	GetTravelTimeMatrixPath() string
	Set(common.InterestMap, common.InterestMap, []common.Vehicle, int, int, []common.Location)
}
