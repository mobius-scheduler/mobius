package main

import (
	"flag"
	"fmt"
	"github.com/mobius-scheduler/apps/aqi"
	"github.com/mobius-scheduler/apps/dynamic"
	"github.com/mobius-scheduler/apps/iperf"
	"github.com/mobius-scheduler/apps/lyft"
	"github.com/mobius-scheduler/apps/parking"
	"github.com/mobius-scheduler/apps/roof"
	"github.com/mobius-scheduler/apps/traffic"
	"github.com/mobius-scheduler/mobius/app"
	"github.com/mobius-scheduler/mobius/common"
	"github.com/mobius-scheduler/mobius/mobius"
	"github.com/mobius-scheduler/mobius/vrp"
	log "github.com/sirupsen/logrus"
	"os"
)

const MAX_ROUNDS = 1000

type Config struct {
	Vehicles       []common.Vehicle `json:"vehicles"`
	Mode           string           `json:"mode"`
	Apps           AppList          `json:"apps"`
	Alpha          float64          `json:"alpha"`
	Discount       float64          `json:"discount"`
	Horizon        int              `json:"horizon"`
	ReplanSec      int              `json:"replan_sec"`
	DurationSec    int              `json:"duration_sec"`
	Capacity       int              `json:"capacity"`
	RTH            int              `json:"rth"`
	Dir            string           `json:"dir"`
	Verbose        bool             `json:"verbose"`
	Hull           bool             `json:"hull"`
	TravelTimePath string           `json:"travel_time_path"`
	Solver         string           `json:"solver"`
}

type AppList []string

func (a *AppList) String() string         { return fmt.Sprintf("%v ", *a) }
func (a *AppList) Set(value string) error { *a = append(*a, value); return nil }

// Create apps by reading from JSON task files
func create_env(alist AppList) []app.Application {
	apps := make([]app.Application, len(alist))
	for i, path := range alist {
		var a app.Application
		var ac app.AppConfig
		common.FromFile(path, &ac)
		switch ac.Type {
		case "dynamic":
			a = &dynamic.AppDynamic{}
		case "aqi":
			a = &aqi.AppAQI{}
		case "iperf":
			a = &iperf.AppIperf{}
		case "parking":
			a = &parking.AppParking{}
		case "traffic":
			a = &traffic.AppTraffic{}
		case "roof":
			a = &roof.AppRoof{}
		case "lyft":
			a = &lyft.AppLyft{}
		default:
			log.Fatalf("[main] app type %v not supported", ac.Type)
		}
		a.Init(ac)
		apps[i] = a
	}

	return apps
}

// Load vehicle from config file and replicate
func load_vehicles(path string, num int) []common.Vehicle {
	if num > 0 {
		var v common.Vehicle
		common.FromFile(path, &v)

		vehicles := make([]common.Vehicle, num)
		for i, _ := range vehicles {
			v.ID = i
			vehicles[i] = v
		}
		return vehicles
	} else {
		var vehicles []common.Vehicle
		common.FromFile(path, &vehicles)
		return vehicles
	}
}

// Get vehicle home location
func get_home(vehicles []common.Vehicle) []common.Location {
	home := make([]common.Location, len(vehicles))
	for i, v := range vehicles {
		home[i] = v.Location
	}
	return home
}

// Merge InterestMaps across apps
func merge_ims(apps []app.Application) common.InterestMap {
	ims := make([]common.InterestMap, len(apps))
	for i, a := range apps {
		ims[i] = a.GetInterestMap()
	}
	return app.MergeInterestMaps(ims)
}

// Create directory to save Mobius logs
func create_dir(path string) {
	if err := os.MkdirAll(path, 0755); err != nil {
		log.Fatalf("[main] error creating directory %s", path)
	}
}

// Save InterestMap, schedule
func save(dir string, im common.InterestMap, s vrp.Schedule, cfg Config) {
	for _, v := range cfg.Vehicles {
		im[common.Task{AppID: -1, Location: v.Location}] = common.TaskData{}
	}
	common.ToFile(dir+"/im.json", im.GetTasks())
	common.ToFile(dir+"/sched.json", s)
}

func main() {
	var cfg Config
	flag.Var(
		&cfg.Apps,
		"app",
		"path to app configs",
	)
	flag.StringVar(
		&cfg.Mode,
		"mode",
		"mobius",
		"scheduler mode (i.e., search, trace, mobius)",
	)
	flag.Float64Var(
		&cfg.Alpha,
		"alpha",
		100.0,
		"alpha value (controls fairness)",
	)
	flag.Float64Var(
		&cfg.Discount,
		"discount",
		1.0,
		"discount factor on historical throughput (1 = no disount)",
	)
	flag.IntVar(
		&cfg.Horizon,
		"horizon",
		360,
		"fairness/planning timescale (seconds)",
	)
	flag.IntVar(
		&cfg.ReplanSec,
		"replan",
		360,
		"replanning interval (seconds)",
	)
	flag.IntVar(
		&cfg.DurationSec,
		"duration",
		0,
		"experiment duration (seconds)",
	)
	flag.IntVar(
		&cfg.Capacity,
		"capacity",
		0,
		"vehicle capacity (objects; 0 = no constraint)",
	)
	flag.IntVar(
		&cfg.RTH,
		"rth",
		900,
		"period at which to return home (seconds)",
	)
	flag.StringVar(
		&cfg.TravelTimePath,
		"ttpath",
		"",
		"path to travel time (distance) matrix",
	)
	flag.StringVar(
		&cfg.Solver,
		"solver",
		"ortools",
		"solver type (ortools, pdptw, gurobi)",
	)
	flag.StringVar(
		&cfg.Dir,
		"dir",
		"",
		"directory to save logs",
	)
	flag.BoolVar(
		&cfg.Hull,
		"hull",
		false,
		"trace hull in each round",
	)
	flag.BoolVar(
		&cfg.Verbose,
		"verbose",
		false,
		"enable verbose logging",
	)
	var vehicles_path = flag.String(
		"cfg_vehicles",
		"vehicles.cfg",
		"path to vehicles config (.cfg) file",
	)
	var vehicles_num = flag.Int(
		"num_vehicles",
		0,
		"number of vehicles (replicate config)",
	)
	flag.Parse()

	// set logging level
	if cfg.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	// load vehicles
	cfg.Vehicles = load_vehicles(*vehicles_path, *vehicles_num)

	// print config
	log.Printf("%+v", cfg)

	// init apps, solver
	apps := create_env(cfg.Apps)
	var solver vrp.Solver
	switch cfg.Solver {
	case "ortools":
		solver = &vrp.GoogleSolver{}
	case "pdptw":
		solver = &vrp.PdptwSolver{}
	default:
		log.Fatalf("[main] solver %v not supported", cfg.Solver)
	}
	if cfg.TravelTimePath != "" {
		solver.SetTravelTimeMatrixPath(cfg.TravelTimePath)
	}

	home := get_home(cfg.Vehicles)

	var rth []common.Location = nil
	if cfg.RTH > 0 {
		rth = home
	}
	var dir string
	switch cfg.Mode {
	case "mobius":
		// create directory
		if cfg.Dir != "" {
			dir = fmt.Sprintf("%s/sprite/alpha%v/", cfg.Dir, cfg.Alpha)
			create_dir(dir)
			common.ToFile(dir+"/config.cfg", cfg)
		}

		// compute number of rounds
		var max_rounds int
		if cfg.DurationSec > 0 {
			max_rounds = int(cfg.DurationSec / cfg.ReplanSec)
		} else {
			max_rounds = MAX_ROUNDS
		}

		// init scheduler and run
		scheduler := mobius.Scheduler{
			Applications: apps,
			Vehicles:     cfg.Vehicles,
			Home:         home,
			Solver:       solver,
			Alpha:        cfg.Alpha,
			Discount:     cfg.Discount,
			Horizon:      cfg.Horizon,
			ReplanSec:    cfg.ReplanSec,
			MaxRounds:    max_rounds,
			Capacity:     cfg.Capacity,
			RTH:          cfg.RTH,
			Dir:          dir,
			Hull:         cfg.Hull,
		}
		scheduler.Run()
	case "trace":
		// create directory
		if cfg.Dir != "" {
			dir = cfg.Dir + "/trace/"
			create_dir(dir)
			common.ToFile(dir+"/config.cfg", cfg)
		}

		// merge IMs, init solver
		im := merge_ims(apps)
		solver.Set(im, im, cfg.Vehicles, cfg.Horizon, cfg.Capacity, rth)

		// init mobius and run
		sp := mobius.Mobius{
			InterestMap: im,
			Solver:      solver,
			Vehicles:    cfg.Vehicles,
			Horizon:     cfg.Horizon,
			Capacity:    cfg.Capacity,
			Alpha:       cfg.Alpha,
			Dir:         dir,
		}
		sp.Init()
		hull := sp.TraceFrontier()
		log.Printf("[main] found hull with allocations: %v", hull)
	case "search":
		// create directory
		if cfg.Dir != "" {
			dir = cfg.Dir + "/search/"
			create_dir(dir)
			common.ToFile(dir+"/config.cfg", cfg)
		}

		// merge IMs, init solver
		im := merge_ims(apps)
		solver.Set(im, im, cfg.Vehicles, cfg.Horizon, cfg.Capacity, rth)

		// init mobius and run
		sp := mobius.Mobius{
			InterestMap: im,
			Solver:      solver,
			Vehicles:    cfg.Vehicles,
			Horizon:     cfg.Horizon,
			Capacity:    cfg.Capacity,
			Alpha:       cfg.Alpha,
		}
		sp.Init()
		sol := sp.SearchFrontier()
		log.Printf("[main] found alpha-fair allocation: %v", sol)

		// write InterestMap, schedule
		if cfg.Dir != "" {
			save(dir, im, sol, cfg)
		}

	default:
		log.Fatalf("[main] mode %s not supported", cfg.Mode)
	}
}
