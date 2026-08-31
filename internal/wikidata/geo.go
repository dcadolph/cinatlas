package wikidata

import "math"

// earthRadiusKM is the mean radius used for great-circle distance.
const earthRadiusKM = 6371.0

// haversine returns the great-circle distance in kilometers between two WKT
// point literals, or zero when either fails to parse. It answers how far a
// production traveled from the place it filmed to the place it claims to be.
func haversine(a, b string) float64 {
	aLat, aLon, ok := parsePoint(a)
	if !ok {
		return 0
	}
	bLat, bLon, ok := parsePoint(b)
	if !ok {
		return 0
	}
	rad := math.Pi / 180
	h := 0.5 - math.Cos((bLat-aLat)*rad)/2 +
		math.Cos(aLat*rad)*math.Cos(bLat*rad)*(1-math.Cos((bLon-aLon)*rad))/2
	return 2 * earthRadiusKM * math.Asin(math.Sqrt(h))
}
