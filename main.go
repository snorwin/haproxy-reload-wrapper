package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/exec"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/log"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/utils"
)

func main() {
	// fetch the absolut path of the haproxy executable
	executable, err := utils.LookupExecutablePathAbs("haproxy")
	if err != nil {
		log.Emergency(err.Error())
		os.Exit(1)
	}

	// execute haproxy with the flags provided as a child process asynchronously
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.AsyncRun(); err != nil {
		log.Emergency(err.Error())
		os.Exit(1)
	}
	log.Notice(fmt.Sprintf("process %d started", cmd.Process.Pid))

	// create a fsnotify.Watcher for config file changes
	cfgFile := utils.LookupHAProxyConfigFile()
	fswatch, err := fsnotify.NewWatcher()
	if err != nil {
		log.Notice(fmt.Sprintf("fsnotify watcher create failed : %v", err))
		os.Exit(1)
	}
	if err := fswatch.Add(cfgFile); err != nil {
		log.Notice(fmt.Sprintf("watch file failed : %v", err))
		os.Exit(1)
	}
	log.Notice(fmt.Sprintf("watch file :  %s", cfgFile))

	// flag used for termination handling
	var terminated bool

	// initialize a signal handler for SIGINT, SIGKILL
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGKILL)

	// endless for loop which handles signals, file system events as well as termination of the child process
	for {
		select {
		case event := <-fswatch.Events:
			if event.Op&fsnotify.Write != fsnotify.Write {
				continue
			}
			log.Notice(fmt.Sprintf("fs event for file %s : %v", cfgFile, event.Op))

			// create a new haproxy process which will replace the old one after it was successfully started
			tmp := exec.Command(executable, append([]string{"-x", utils.LookupHAProxySocketPath(), "-sf", strconv.Itoa(cmd.Process.Pid)}, os.Args[1:]...)...)
			tmp.Stdout = os.Stdout
			tmp.Stderr = os.Stderr
			if err := tmp.AsyncRun(); err != nil {
				log.Warning(err.Error())
				log.Warning("reload failed")

				continue
			}
			log.Notice(fmt.Sprintf("process %d started", tmp.Process.Pid))

			select {
			case <-cmd.Terminated:
				// old haproxy terminated - successfully started a new process replacing the old one
				log.Notice(fmt.Sprintf("process %d termianted : %s", cmd.Process.Pid, cmd.Status()))
				log.Notice("reload successful")
				cmd = tmp
			case <-tmp.Terminated:
				// new haproxy terminated without terminating the old process - this can happen if the modified configuration file was invalid
				log.Warning(fmt.Sprintf("process %d termianted unexpectedly : %s", tmp.Process.Pid, tmp.Status()))
				log.Warning("reload failed")
			}
		case err := <-fswatch.Errors:
			// handle errors of fsnotify.Watcher
			log.Alert(err.Error())
		case sig := <-sigs:
			// handle SIGINT, SIGKILL and propagate it to child process
			log.Notice(fmt.Sprintf("recived singal %d", sig))

			if cmd.Process == nil {
				// received termination suddenly before child process was even started
				os.Exit(0)
			}

			// set termination flag before propagating the signal in order to prevent race conditions
			terminated = true

			// propagate signal to child process
			if err := cmd.Process.Signal(sig); err != nil {
				log.Warning(fmt.Sprintf("propagating signal %d to process %d failed", sig, cmd.Process.Pid))
			}
		case <-cmd.Terminated:
			// check for unexpected termination
			if !terminated {
				log.Emergency(fmt.Sprintf("proccess %d teminated unexpectedly : %s", cmd.Process.Pid, cmd.Status()))
				if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
					os.Exit(cmd.ProcessState.ExitCode())
				} else {
					os.Exit(1)
				}
			}

			log.Notice(fmt.Sprintf("process %d termianted : %s", cmd.Process.Pid, cmd.Status()))
			os.Exit(cmd.ProcessState.ExitCode())
		}
	}
}
