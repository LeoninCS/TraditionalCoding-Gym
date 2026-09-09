package main

import "sync"

type Result struct {
	Value int
	Err   error
}

func Map(input []int, workers int, fn func(int) (int, error)) []Result {
	if workers <= 0 || fn == nil {
		panic("panic")
	}

	if len(input) == 0 {
		return nil
	}

	ans := make([]Result, len(input))
	var wg sync.WaitGroup
	wg.Add(len(input))
	ch := make(chan int, min(workers, len(input)))

	for i := range input {
		ch <- 1
		go func() {
			defer wg.Done()
			defer func() { <-ch }()
			ans[i].Value, ans[i].Err = fn(input[i])
		}()
	}
	wg.Wait()
	return ans
}

func main() {
	Map([]int{1, 2, 3, 4, 5, 6}, 3, func(x int) (int, error) {
		return x * x, nil
	})
}
