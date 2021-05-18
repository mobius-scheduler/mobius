package vrp

import (
	"github.com/mobius-scheduler/mobius/common"
	log "github.com/sirupsen/logrus"
)

// dedicate vehicle solver
type DedicatePdptwSolver struct {
	solver                  Solver
	interest_map            common.InterestMap
	vehicles                []common.Vehicle
	budget                  int
	capacity                int
	unweighted_interest_map common.InterestMap
	initial_schedule        Schedule
	rth                     []common.Location
	app_ids                 []int
	vehicles_per_app        int
	travel_time_matrix_path string
}

func (d *DedicatePdptwSolver) New() Solver {
	return &DedicatePdptwSolver{}
}

func (d *DedicatePdptwSolver) SetInterestMap(im common.InterestMap) {
	d.interest_map = im
}

func (d *DedicatePdptwSolver) GetInterestMap() common.InterestMap {
	return d.interest_map
}

func (d *DedicatePdptwSolver) GetRTH() []common.Location {
	return d.rth
}

func (d *DedicatePdptwSolver) SetInitialSchedule(s Schedule) {
	d.initial_schedule = s
}

func (d *DedicatePdptwSolver) SetTravelTimeMatrixPath(p string) {
	d.travel_time_matrix_path = p
}

func (d *DedicatePdptwSolver) GetTravelTimeMatrixPath() string {
	return d.travel_time_matrix_path
}

func (d *DedicatePdptwSolver) Set(im, uim common.InterestMap, v []common.Vehicle, b, c int, r []common.Location) {
	d.interest_map = im
	d.unweighted_interest_map = uim
	d.vehicles = v
	d.budget = b
	d.capacity = c
	d.rth = r
	d.app_ids = im.GetApps()

	// check if dedicating is possible
	if len(d.vehicles)%len(d.app_ids) != 0 {
		log.Warnf(
			"[vrp] dedicating vehicle/app not possible: %d apps, %d vehicles",
			len(d.app_ids),
			len(d.vehicles),
		)
	}
	d.vehicles_per_app = int(len(d.vehicles) / len(d.app_ids))
}

func (d *DedicatePdptwSolver) Solve() Schedule {
	schedules := make([]Schedule, len(d.app_ids))
	for i, id := range d.app_ids {
		// setup interestmap, vehicles
		ima := d.interest_map.FilterByApp(id)
		v := d.vehicles[i*d.vehicles_per_app : (i+1)*d.vehicles_per_app]
		var r []common.Location = nil
		if d.rth != nil {
			r = d.rth[i*d.vehicles_per_app : (i+1)*d.vehicles_per_app]
		}

		solver := NewPdptwSolver(ima, ima, v, d.budget, d.capacity, r)
		solver.SetTravelTimeMatrixPath(d.travel_time_matrix_path)
		schedules[i] = solver.Solve()
	}

	// merge schedules
	var master_schedule Schedule
	master_schedule.Allocation = make(Allocation)
	for i, id := range d.app_ids {
		master_schedule.Routes = append(
			master_schedule.Routes,
			schedules[i].Routes...,
		)
		master_schedule.Allocation[id] = schedules[i].Allocation[id]
	}
	master_schedule.Stats.Alpha = -1
	return master_schedule
}
