package vrp

import (
	"github.com/mobius-scheduler/mobius/common"
	"math"
)

const EARTH_RADIUS = 6.3781 * 1e6

func travel_time(src, dst common.Location, speed float64, task_time float64) int {
	dx := (dst.Longitude - src.Longitude) *
		math.Cos(0.5*(src.Latitude+dst.Latitude)*math.Pi/180) * math.Pi / 180 * EARTH_RADIUS
	dy := (dst.Latitude - src.Latitude) * math.Pi / 180 * EARTH_RADIUS
	dist := math.Sqrt(math.Pow(dx, 2) + math.Pow(dy, 2))
	flight_time := dist / speed
	return int(math.Ceil(flight_time + task_time))
}
