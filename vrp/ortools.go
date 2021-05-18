package vrp

import (
	"bytes"
	"encoding/json"
	"github.com/mobius-scheduler/mobius/common"
	"log"
	"os"
	"os/exec"
)

// wrapper for ORTools solver (written in Python)
type GoogleSolver struct {
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

func (g *GoogleSolver) New() Solver {
	return &GoogleSolver{}
}

func NewGoogleSolver(im, uim common.InterestMap, v []common.Vehicle, b, c int, r []common.Location) *GoogleSolver {
	s := &GoogleSolver{}
	s.Set(im, uim, v, b, c, r)
	return s
}

func (g *GoogleSolver) SetInterestMap(im common.InterestMap) {
	g.interest_map = im
}

func (g *GoogleSolver) GetInterestMap() common.InterestMap {
	return g.interest_map
}

func (g *GoogleSolver) GetRTH() []common.Location {
	return g.rth
}

func (g *GoogleSolver) SetInitialSchedule(s Schedule) {
	g.initial_schedule = s
}

func (g *GoogleSolver) SetTravelTimeMatrixPath(p string) {
	g.travel_time_matrix_path = p
}

func (g *GoogleSolver) GetTravelTimeMatrixPath() string {
	return g.travel_time_matrix_path
}

func (g *GoogleSolver) Set(im, uim common.InterestMap, v []common.Vehicle, b, c int, r []common.Location) {
	g.interest_map = im
	g.unweighted_interest_map = uim
	g.vehicles = v
	g.budget = b
	g.capacity = c
	g.rth = r
}

func (g *GoogleSolver) Solve() Schedule {
	// create InterestMap, Vehicle JSONs
	inp := Input{
		InterestMap:           g.interest_map.ToFile(),
		UnweightedInterestMap: g.unweighted_interest_map.ToFile(),
		Vehicles:              g.vehicles,
		Budget:                g.budget,
		Capacity:              g.capacity,
		InitialSchedule:       g.initial_schedule,
		TravelTimeMatrixPath:  g.travel_time_matrix_path,
		RTH:                   g.rth,
	}
	inpj := common.ToJSON(inp)

	// run solver
	cmd := exec.Command("python3", "solvers/vrp_ortools.py")
	cmd.Dir = common.GetDir()
	var inpbuf, outbuf bytes.Buffer
	inpbuf.Write(inpj)
	cmd.Stdin = &inpbuf
	cmd.Stdout = &outbuf
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("[vrp] error running ortools: %v", err)
	}

	var schedule Schedule
	if err := json.Unmarshal(outbuf.Bytes(), &schedule); err != nil {
		log.Fatalf("[vrp] error unmarshaling json to output struct: %v", err)
	}

	return schedule
}
