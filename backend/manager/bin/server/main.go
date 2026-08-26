package main

import (
	"os"

	"github.com/tbdavid2019/888a2a/backend/manager/bin/server/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
