package main

import "time"

const (
	fileTempSuffix = ".tmp"
)

const (
	ParallelCollectorsLimit = 16
	sshTimeOut              = time.Duration(15 * time.Second)
)

const PSKLen = 16
