package types

func maxSourceInventoryAdvisoryTotal(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
