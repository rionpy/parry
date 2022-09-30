/*
Copyright © 2022 Martti Leino <rionpy@gmail.com>
GNU General Public License v3.0+ (see LICENSE or https://www.gnu.org/licenses/gpl-3.0.txt)
*/
package main

import (
	"fmt"
	"os"
	"parry/lib"
	"strings"
)

func printHelp() {
	fmt.Println("Hello!")
}

func main() {
	config := lib.Config{}
	latestIndex := -1
	args := os.Args[1:]
	for i, arg := range args {
		if i == latestIndex {
			continue
		}
		flag := arg
		equals := strings.IndexByte(arg, '=')
		if equals > -1 {
			flag = arg[:equals]
		}
		switch flag {
		case `-h`, `--help`:
			printHelp()
			os.Exit(0)
		case `-l`, `--list`:
			config.SetList()
		case `-p`, `--preserve`:
			config.SetPreserve()
		case `--ignoreQuotes`:
			config.SetIgnore()
		case `--interpret`:
			config.SetInterpret("foo")
		case `-i`:
			config.SetEditInPlace()
		case `-e`, `--env`:
			if flag == arg {
				latestIndex = i + 1
				config.AddOverride(args[latestIndex])
			} else {
				config.AddOverride(arg[equals+1:])
			}
		case `--envfile`:
			if flag == arg {
				latestIndex = i + 1
				config.AddEnvFile(args[latestIndex])
			} else {
				config.AddEnvFile(arg[equals+1:])
			}
		default:
			if i == len(args)-1 {
				config.AddFile(arg)
			}
		}
	}

	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()

	config.Validate()

	lib.GetOutput(config)
}
