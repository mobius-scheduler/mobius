package mobius

import (
	"fmt"
	"github.com/mobius-scheduler/mobius/app"
	"github.com/mobius-scheduler/mobius/common"
	"github.com/mobius-scheduler/mobius/vrp"
	log "github.com/sirupsen/logrus"
)

// schema for Mobius scheduler
type Scheduler struct {
	Applications []app.Application
	Vehicles     []common.Vehicle
	Home         []common.Location
	Solver       vrp.Solver
	Alpha        float64
	Discount     float64
	Horizon      int
	ReplanSec    int
	MaxRounds    int
	Capacity     int
	RTH          int
	Dir          string
	Hull         bool
	interest_map common.InterestMap
	allocation   vrp.Allocation
}

// merge interest maps from all apps
// cap task by capacity
func (s *Scheduler) get_interest_map() (common.InterestMap, common.InterestMap) {
	ims := make([]common.InterestMap, len(s.Applications))
	for i, a := range s.Applications {
		ims[i] = a.GetInterestMap()
	}
	im_all := app.MergeInterestMaps(ims)

	// enqueue tasks that violate capacity constraints
	// add if there's balance
	im := make(common.InterestMap)
	for t, d := range im_all {
		if s.Capacity > 0 && int(d.Interest) > s.Capacity {
			d.Interest = float64(s.Capacity)
			d.TaskTimeSeconds = float64(s.Capacity)
		}
		im[t] = d
	}

	return im_all, im
}

// update vehicle positions
func (s *Scheduler) update_vehicles(schedule vrp.Schedule) {
	for i, route := range schedule.Routes {
		s.Vehicles[i].Location = route.VehicleEnd
	}
}

// inform apps of completed tasks
func (s *Scheduler) update_apps(schedule vrp.Schedule, im common.InterestMap, time int) {
	// build map of completed tasks (by app)
	app_tasks := make(map[int][]common.TaskData)
	for _, route := range schedule.Routes {
		for _, task := range route.Path {
			task.FulfillTime = time + task.FulfillTime
			if task.Destination.Longitude != common.INVALID_LOC && task.Destination.Latitude != common.INVALID_LOC {
				app_tasks[task.AppID] = append(app_tasks[task.AppID], task)
			}
		}
	}

	// update apps
	for _, app := range s.Applications {
		app.Update(app_tasks[app.GetID()], time+s.ReplanSec)
	}
}

// run Mobius for multiple rounds
func (s *Scheduler) Run() {
	s.allocation = make(vrp.Allocation)
	im_all, im := s.get_interest_map()
	round := 0
	budget_time := 0
	total_time := 0

	// init Mobius
	sp := Mobius{
		InterestMap: im,
		Solver:      s.Solver,
		Vehicles:    s.Vehicles,
		Horizon:     s.Horizon,
		Capacity:    s.Capacity,
		Historical:  s.allocation,
		Alpha:       s.Alpha,
		Discount:    s.Discount,
	}

	// run scheduler in loop
	for len(im_all) > 0 && round < s.MaxRounds {
		// prepare solver, mobius
		var rth []common.Location = nil
		if s.RTH > 0 && budget_time+s.Horizon >= s.RTH {
			log.Printf("[mobius] round %d, rth enabled", round)
			rth = s.Home
			budget_time = 0
		}
		total := 0.0
		for _, a := range s.Applications {
			total += a.GetInterestMap().GetTotalInterest()
		}
		log.Printf(
			"[mobius] %v customers, %v tasks, %v vehicles",
			len(s.Applications),
			total,
			len(s.Vehicles),
		)

		// update solver, scheduler params
		s.Solver.Set(im, im, s.Vehicles, s.Horizon, s.Capacity, rth)
		s.Solver.SetInitialSchedule(vrp.Schedule{})
		sp.InterestMap = im
		sp.Solver = s.Solver
		sp.Vehicles = s.Vehicles
		sp.Historical = s.allocation

		// find schedule
		// run mobius if alpha > 0, otherwise use solver
		var schedule vrp.Schedule
		var hull []vrp.Schedule
		if s.Alpha > 0 {
			sp.Init()
			schedule = sp.SearchFrontier()
			if s.Hull {
				hull = sp.TraceFrontier()
			}
		} else if s.Alpha == 0 {
			schedule = s.Solver.Solve()
		} else if s.Alpha == -1 {
			var d vrp.Solver
			switch x := (sp.Solver).(type) {
			case *vrp.GoogleSolver:
				d = &vrp.DedicateSolver{}
				d.Set(
					sp.InterestMap,
					sp.InterestMap,
					sp.Vehicles,
					sp.Horizon,
					sp.Capacity,
					rth,
				)
				schedule = d.Solve()
			case *vrp.PdptwSolver:
				d = &vrp.DedicatePdptwSolver{}
				d.Set(
					sp.InterestMap,
					sp.InterestMap,
					sp.Vehicles,
					sp.Horizon,
					sp.Capacity,
					rth,
				)
				d.SetTravelTimeMatrixPath(sp.Solver.GetTravelTimeMatrixPath())
				schedule = d.Solve()
			default:
				log.Fatalf("[mobius] solver %v not supported", x)
			}
		} else if s.Alpha == -2 {
			r := vrp.RoundRobinSolver{}
			r.Set(
				sp.InterestMap,
				sp.InterestMap,
				sp.Vehicles,
				sp.Horizon,
				sp.Capacity,
				rth,
			)
			schedule = r.Solve()
		}

		log.Printf(
			"[mobius] time %d-%d, allocation %+v",
			total_time,
			total_time+s.Horizon,
			schedule.Allocation,
		)

		// trim schedule
		schedule.Trim(s.ReplanSec)

		// save interestmap, schedule
		if s.Dir != "" {
			common.ToFile(
				fmt.Sprintf("%s/im_round%04d.json", s.Dir, round),
				im_all.ToFile(),
			)
			common.ToFile(
				fmt.Sprintf("%s/schedule_round%04d.json", s.Dir, round),
				schedule,
			)
			common.ToFile(
				fmt.Sprintf("%s/hull_round%04d.json", s.Dir, round),
				hull,
			)
		}

		// update cumulative allocation
		total_alloc := 0.0
		for id, a := range schedule.Allocation {
			s.allocation[id] += a
			total_alloc += a
		}
		log.Printf(
			"[mobius] time %d-%d, round %d, allocation %+v",
			total_time,
			total_time+s.ReplanSec,
			round,
			schedule.Allocation,
		)

		log.Printf("round %d, cumulative allocation: %v", round, s.allocation)

		// update vehicle positions, applications
		s.update_vehicles(schedule)
		s.update_apps(schedule, im, total_time)

		// update elapsed time
		budget_time += s.ReplanSec
		total_time += s.ReplanSec

		// update im
		im_all, im = s.get_interest_map()
		round++
	}
}
