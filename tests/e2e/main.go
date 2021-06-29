package main

import "fmt"

func add_nums(a int, b int) int {
	sum := a + b
	return sum
}

func main() {
	fmt.Println(add_nums(3, 6))
}
