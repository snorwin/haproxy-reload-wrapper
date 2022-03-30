package log

import (
	"fmt"
	"os"
)

const (
	LevelEmergency = "EMERGENCY"
	LevelAlert     = "ALERT"
	LevelWarning   = "WARNING"
	LevelNotice    = "NOTICE"
)

func Emergency(msg string) {
	log(LevelEmergency, os.Getpid(), msg)
}

func Alert(msg string) {
	log(LevelAlert, os.Getpid(), msg)
}

func Warning(msg string) {
	log(LevelWarning, os.Getpid(), msg)
}

func Notice(msg string) {
	log(LevelNotice, os.Getpid(), msg)
}

func log(level string, pid int, msg string) {
	fmt.Printf("%-10s (%d) : %s\n", fmt.Sprintf("[%s]", level), pid, msg)
}
