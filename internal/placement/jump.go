package placement

// r is a random fraction between 0 and 1 that is never 0
func jumpBucket(key uint64, buckets int) int {
	b, j := int64(-1), int64(0)
	for j < int64(buckets) {
		b = j
		key = key*2862933555777941757 + 1
		r := float64((key>>33)+1) / float64(1<<31)
		j = int64(float64(b+1) / r)
	}
	return int(b)
}
