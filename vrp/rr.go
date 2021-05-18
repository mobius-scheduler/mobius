package vrp

import (
	"github.com/mobius-scheduler/mobius/common"
	"sort"
)

// round-robin scheduler
type RoundRobinSolver struct {
	solver       Solver
	interest_map common.InterestMap
	vehicles     []common.Vehicle
	budget       int
	rth          []common.Location
}

func (r *RoundRobinSolver) SetInterestMap(im common.InterestMap) {
	r.interest_map = im
}

func (r *RoundRobinSolver) GetInterestMap() common.InterestMap {
	return r.interest_map
}

func (r *RoundRobinSolver) GetRTH() []common.Location { return r.rth }

func (r *RoundRobinSolver) SetInitialSchedule(s Schedule) {}

func (r *RoundRobinSolver) Set(im, uim common.InterestMap, v []common.Vehicle, b, c int, x []common.Location) {
	r.interest_map = im
	r.vehicles = v
	r.budget = b
	r.rth = x
}

func (r *RoundRobinSolver) next_task(v common.Vehicle, im common.InterestMap, app_id int) (common.TaskData, int) {
	ima := im.FilterByApp(app_id)
	type rr_task struct {
		task        common.TaskData
		travel_time int
	}

	tasks := make([]rr_task, len(ima))
	i := 0
	for task, data := range ima {
		tt := travel_time(v.Location, task.Location, v.Speed, data.TaskTimeSeconds)
		tasks[i] = rr_task{
			task:        data,
			travel_time: tt,
		}
		i++
	}

	// sort tasks by min travel_time
	sort.Slice(
		tasks, func(i, j int) bool { return tasks[i].travel_time < tasks[j].travel_time },
	)
	return tasks[0].task, tasks[0].travel_time
}

func (r *RoundRobinSolver) travel_time_home(v common.Vehicle, h common.Location, loc common.Location) int {
	return travel_time(loc, h, v.Speed, 0)
}

func (r *RoundRobinSolver) Solve() Schedule {
	// create copy of im
	im := make(common.InterestMap)
	for t, d := range r.interest_map {
		im[t] = d
	}

	// create copy of vehicles
	v := make([]common.Vehicle, len(r.vehicles))
	for i, x := range r.vehicles {
		v[i] = x
	}

	// get app ids, to shuffle through apps
	app_ids := r.interest_map.GetApps()

	var s Schedule
	s.Allocation = make(Allocation)
	for i, vehicle := range v {
		var path []common.TaskData
		var interest float64
		var time int
		start := vehicle.Location
	out:
		for time <= r.budget {
			for _, app := range app_ids {
				next, tt := r.next_task(vehicle, im, app)
				var th int
				if r.rth != nil {
					th = r.travel_time_home(vehicle, r.rth[i], next.Location)
				} else {
					th = 0
				}
				if time+tt+th >= r.budget {
					break out
				}
				path = append(path, next)
				interest += r.interest_map[next.GetTask()].Interest
				time += tt
				s.Allocation[app] += r.interest_map[next.GetTask()].Interest
				vehicle.Location = next.Location
				delete(im, next.GetTask())
			}
		}
		var end common.Location
		if r.rth != nil {
			last := path[len(path)-1].Location
			end = r.rth[i]
			time += r.travel_time_home(vehicle, end, last)
		} else {
			end = path[len(path)-1].Location
		}
		s.Routes = append(
			s.Routes,
			Route{
				Path:          path,
				TotalInterest: interest,
				TotalTime:     time,
				VehicleStart:  start,
				VehicleEnd:    end,
			},
		)
	}
	s.Stats.Alpha = -2
	return s
}
