package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/seb/omarchy-plugin-steam/internal/steamstats"
)

func main() {
	if len(os.Args) != 3 {
		out, _ := json.Marshal(steamstats.Result{
			OK:    false,
			Error: "usage: steam-library-stats <steam_dir> <localconfig>",
		})
		fmt.Println(string(out))
		os.Exit(1)
	}

	result, err := steamstats.Compute(os.Args[1], os.Args[2])
	if err != nil {
		out, _ := json.Marshal(steamstats.Result{OK: false, Error: err.Error()})
		fmt.Println(string(out))
		return
	}

	out, err := json.Marshal(result)
	if err != nil {
		out, _ = json.Marshal(steamstats.Result{OK: false, Error: err.Error()})
		fmt.Println(string(out))
		return
	}
	fmt.Println(string(out))
}
