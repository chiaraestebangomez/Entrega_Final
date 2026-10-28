package geo

import (
	"fmt"
	"math"
	"strings"
)

type Coord struct {
	Lat float64
	Lng float64
}

var known = map[string]Coord{
	"buenos aires":     {-34.6037, -58.3816},
	"cordoba":          {-31.4201, -64.1888},
	"mendoza":          {-32.8895, -68.8458},
	"bariloche":        {-41.1335, -71.3103},
	"santiago":         {-33.4489, -70.6693},
	"lima":             {-12.0464, -77.0428},
	"bogota":           {4.7110, -74.0721},
	"ciudad de mexico": {19.4326, -99.1332},
	"miami":            {25.7617, -80.1918},
	"nueva york":       {40.7128, -74.0060},
	"new york":         {40.7128, -74.0060},
	"madrid":           {40.4168, -3.7038},
	"barcelona":        {41.3874, 2.1686},
	"paris":            {48.8566, 2.3522},
	"roma":             {41.9028, 12.4964},
	"londres":          {51.5074, -0.1278},
	"lisboa":           {38.7223, -9.1393},
	"tokio":            {35.6762, 139.6503},
	"sydney":           {-33.8688, 151.2093},
}

func Lookup(destination string) (Coord, bool) {
	key := strings.ToLower(strings.TrimSpace(destination))
	if c, ok := known[key]; ok {
		return c, true
	}
	for name, c := range known {
		if strings.Contains(key, name) {
			return c, true
		}
	}
	return Coord{}, false
}

func DistanceKM(a, b Coord) float64 {
	const earthRadius = 6371.0
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(h))
}

func NavigationURL(from Coord, to Coord) string {
	return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&origin=%.6f,%.6f&destination=%.6f,%.6f",
		from.Lat, from.Lng, to.Lat, to.Lng)
}
