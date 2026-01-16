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

var (
	executable string
	cmds       map[int]*exec.Cmd
)

func main() {
	// fetch the absolut path of the haproxy executable
	var err error
	executable, err = utils.LookupExecutablePathAbs("haproxy")
	if err != nil {
		log.Emergency(err.Error())
		os.Exit(1)
	}

	cmds = make(map[int]*exec.Cmd)

	// execute haproxy with the flags provided as a child process asynchronously
	runInstance()

	watchPath := utils.LookupWatchPath()
	if watchPath == "" {
		watchPath = utils.LookupHAProxyConfigFile()
	}

	// create a fsnotify.Watcher for config changes
	fswatch, err := fsnotify.NewWatcher()
	if err != nil {
		log.Notice(fmt.Sprintf("fsnotify watcher create failed : %v", err))
		os.Exit(1)
	}

	if utils.DisableReload() {
		log.Notice("reload disabled, no watches added")
	} else {
		if err := fswatch.Add(watchPath); err != nil {
			log.Notice(fmt.Sprintf("watch failed : %v", err))
			os.Exit(1)
		}
		log.Notice(fmt.Sprintf("watch : %s", watchPath))
	}

	// initialize a signal handler for SIGINT, SIGTERM and SIGUSR1 (for OpenShift)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	// endless for loop which handles signals, file system events as well as termination of the child process
	for {
		select {
		case event := <-fswatch.Events:
			// only care about events which may modify the contents of the directory
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Remove) && !event.Has(fsnotify.Create) {
				continue
			}

			log.Notice(fmt.Sprintf("fs event for %s : %v", watchPath, event.Op))

			// re-add watch if path was removed - config maps are updated by removing/adding a symlink
			if event.Has(fsnotify.Remove) {
				if err := fswatch.Add(watchPath); err != nil {
					log.Alert(fmt.Sprintf("watch failed : %v", err))
				} else {
					log.Notice(fmt.Sprintf("watch : %s", watchPath))
				}
			}

			// create a new haproxy process which will take over listeners
			// from the existing one one after it was successfully started
			runInstance()

		case err := <-fswatch.Errors:
			// handle errors of fsnotify.Watcher
			log.Alert(err.Error())
		case sig := <-sigs:
			// handle SIGINT, SIGTERM, SIGUSR1 and propagate it to child process
			log.Notice(fmt.Sprintf("received singal %d", sig))

			if len(cmds) == 0 {
				// received termination suddenly before child process was even started
				os.Exit(0)
			}

			// propagate signal to child processes
			for i := range cmds {
				if cmds[i].Process != nil {
					if err := cmds[i].Process.Signal(sig); err != nil {
						log.Warning(fmt.Sprintf("propagating signal %d to process %d failed", sig, cmds[i].Process.Pid))
					}
				}
			}
		}
	}
}

func runInstance() error {
	args := os.Args[1:]
	if len(cmds) > 0 {
		args = append(args, []string{"-x", utils.LookupHAProxySocketPath(), "-sf", pids(cmds)}...)
	}
	cmd := exec.Command(executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = utils.LoadEnvFile()

	if err := cmd.AsyncRun(); err != nil {
		log.Warning(err.Error())
		log.Warning("reload failed")
		return err
	}
	go func(cmd *exec.Cmd) {
		<-cmd.Terminated
		log.Notice(fmt.Sprintf("process %d terminated : %s", cmd.Process.Pid, cmd.Status()))
		delete(cmds, cmd.Process.Pid)

		if len(cmds) == 0 {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
				os.Exit(cmd.ProcessState.ExitCode())
			} else {
				os.Exit(0)
			}
		}
	}(cmd)

	log.Notice(fmt.Sprintf("started process with pid %d and status %s", cmd.Process.Pid, cmd.Status()))
	cmds[cmd.Process.Pid] = cmd
	return nil
}

func pids(m map[int]*exec.Cmd) string {
	var str string
	if len(m) == 0 {
		return str
	}

	for k := range m {
		str = strconv.Itoa(k) + " " + str
	}

	return str
}
