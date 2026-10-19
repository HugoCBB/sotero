package main

import "fmt"

func main() {
	grafo := map[string][]string{
		"ASML":     {"TSMC"},
		"TSMC":     {"NVIDIA"},
		"SK Hynix": {"NVIDIA"},
		"NVIDIA":   {"Microsoft"},
	}

	for _, item := range grafo {
		fmt.Println(item)	}

}