package combination

func combine(n, k int) [][]int {
	if n <= 0 || k <= 0 || k > n {
		return [][]int{}
	}

	result := [][]int{}
	backtrack(&result, []int{}, 1, n, k)
	return result
}

func backtrack(result *[][]int, current []int, start, n, k int) {
	if k == 0 {
		temp := make([]int, len(current))
		copy(temp, current)
		*result = append(*result, temp)
		return
	}

	for i := start; i <= n-k+1; i++ {
		backtrack(result, append(current, i), i+1, n, k-1)
	}
}
