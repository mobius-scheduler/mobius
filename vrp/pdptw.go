package vrp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/mobius-scheduler/mobius/common"
	log "github.com/sirupsen/logrus"
	"os"
	"os/exec"
	"time"
)

// wrapper for pdptw solver (written in Python)
type PdptwSolver struct {
	solver                  Solver
	interest_map            common.InterestMap
	vehicles                []common.Vehicle
	budget                  int
	capacity                int
	unweighted_interest_map common.InterestMap
	initial_schedule        Schedule
	rth                     []common.Location
	travel_time_matrix_path string
}

func (g *PdptwSolver) New() Solver {
	return &PdptwSolver{}
}

func NewPdptwSolver(im, uim common.InterestMap, v []common.Vehicle, b, c int, r []common.Location) *PdptwSolver {
	s := &PdptwSolver{}
	s.Set(im, uim, v, b, c, r)
	return s
}

func (g *PdptwSolver) SetInterestMap(im common.InterestMap) {
	g.interest_map = im
}

func (g *PdptwSolver) GetInterestMap() common.InterestMap {
	return g.interest_map
}

func (g *PdptwSolver) GetRTH() []common.Location {
	return g.rth
}

func (g *PdptwSolver) SetInitialSchedule(s Schedule) {
	g.initial_schedule = s
}

func (g *PdptwSolver) SetTravelTimeMatrixPath(p string) {
	g.travel_time_matrix_path = p
}

func (g *PdptwSolver) GetTravelTimeMatrixPath() string {
	return g.travel_time_matrix_path
}

func (g *PdptwSolver) Set(im, uim common.InterestMap, v []common.Vehicle, b, c int, r []common.Location) {
	g.interest_map = im
	g.unweighted_interest_map = uim
	g.vehicles = v
	g.budget = b
	g.capacity = c
	g.rth = r
}

func (g *PdptwSolver) to_txt() string {
	var out string
	var idx int
	node_map := make(map[common.Task]int)

	// tt matrix path
	out += fmt.Sprintf("%s\n", g.travel_time_matrix_path)

	// header
	out += fmt.Sprintf("%d\t%d\t%0.1f\t%d\n", len(g.vehicles), g.capacity, g.vehicles[0].Speed, g.budget)

	// vehicles
	for _, v := range g.vehicles {
		out += fmt.Sprintf(
			"%d\t-1\t0\t%0.6f\t%0.6f\t0\t0\t0\t0\t0\t0\n",
			idx, v.Location.Latitude, v.Location.Longitude,
		)
		idx += 1
	}

	// tasks
	interest := 0.0
	for k, task := range g.interest_map {
		out += fmt.Sprintf(
			"%d\t%d\t%d\t%0.6f\t%0.6f\t%d\t%0.4f\t%0.4f\t%d\t%d\t%d\n",
			idx, task.AppID, task.RequestTime, task.Location.Latitude, task.Location.Longitude,
			1, task.Interest, g.unweighted_interest_map[k].Interest, task.FulfillTime, 0, idx+1)
		node_map[k] = idx
		out += fmt.Sprintf(
			"%d\t%d\t%d\t%0.6f\t%0.6f\t%d\t%0.4f\t%0.4f\t%d\t%d\t%d\n",
			idx+1, task.AppID, task.RequestTime, task.Destination.Latitude, task.Destination.Longitude,
			-1, task.Interest, g.unweighted_interest_map[k].Interest, task.FulfillTime, idx, 0)
		t := common.Task{
			AppID:       task.AppID,
			Location:    task.Destination,
			Destination: common.Location{common.INVALID_LOC, common.INVALID_LOC},
			RequestTime: task.RequestTime,
		}
		node_map[t] = idx + 1
		idx += 2
		interest += task.Interest
	}

	// initial schedule
	for _, r := range g.initial_schedule.Routes {
		out += fmt.Sprintf("-1\t")
		for i, t := range r.Path {
			task := t.GetTask()
			if _, exists := node_map[task]; !exists {
				log.Fatalf("task %+v invalid", task)
			}

			if i > 0 && node_map[r.Path[i-1].GetTask()] == node_map[r.Path[i].GetTask()] {
				log.Fatalf("cannot stay at same node: task %+v --> task %+v", r.Path[i-1], r.Path[i])
			}

			if i < len(r.Path)-1 {
				out += fmt.Sprintf("%d\t", node_map[task])
			} else {
				out += fmt.Sprintf("%d\n", node_map[task])
			}
		}
	}

	return out
}

func (g *PdptwSolver) Solve() Schedule {
	// create txt for problem
	inp := []byte(g.to_txt())

	// run solver
	cmd := exec.Command("./solvers/or-tools/bin/pdptw")
	cmd.Dir = common.GetDir()
	var inpbuf, outbuf bytes.Buffer
	inpbuf.Write(inp)
	cmd.Stdin = &inpbuf
	cmd.Stdout = &outbuf
	cmd.Stderr = os.Stderr

	start := time.Now()
	if err := cmd.Run(); err != nil {
		log.Fatalf("[vrp] error running ortools: %v", err)
	}
	end := time.Now()
	log.Debugf("[vrp] solver took %v seconds", end.Sub(start).Seconds())

	var schedule Schedule
	if err := json.Unmarshal(outbuf.Bytes(), &schedule); err != nil {
		log.Fatalf("[vrp] error unmarshaling json to output struct: %v", err)
	}

	return schedule
}
