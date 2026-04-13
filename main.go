package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/exec"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/log"
	"github.com/snorwin/haproxy-reload-wrapper/pkg/utils"
)

var (
	executable   string
	processes    []Process
	l            sync.RWMutex
	terminated   bool
	shutdownWait *time.Duration
)

type Process struct {
	cmd           *exec.Cmd
	terminateTime *time.Time
}

func main() {
	// fetch the absolut path of the haproxy executable
	var err error
	executable, err = utils.LookupExecutablePathAbs("haproxy")
	if err != nil {
		log.Emergency(err.Error())
		os.Exit(1)
	}

	shutdownWait = utils.ShutdownWait()
	log.Notice(fmt.Sprintf("shutdown wait duration : %s", shutdownWait))

	// execute haproxy with the flags provided as a child process
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
		log.Notice("reload disabled : no watches added")
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
			// from the previous ones after it was successfully started
			runInstance()

			// remove old haproxy processes if SHUTDOWN_WAIT has been specified
			cleanupInstances()

		case err := <-fswatch.Errors:
			// handle errors of fsnotify.Watcher
			log.Alert(err.Error())
		case sig := <-sigs:
			// handle SIGINT, SIGTERM, SIGUSR1 and propagate it to child process
			log.Notice(fmt.Sprintf("received singal : %d", sig))

			if len(processes) == 0 {
				// received termination suddenly before child process was even started
				os.Exit(0)
			}

			// set termination flag before propagating the signal in order to prevent race conditions
			terminated = true

			// propagate signal to child processes
			for i := range processes {
				if processes[i].cmd.Process != nil {
					if err := processes[i].cmd.Process.Signal(sig); err != nil {
						log.Warning(fmt.Sprintf("propagating signal %d to process %d failed : %s", sig, processes[i].cmd.Process.Pid, err.Error()))
					}
				}
			}
		}
	}
}

func runInstance() {

	log.Notice("starting validation")

	// validate the config by using the "-c" flag
	argsValidate := append(os.Args[1:], "-c")
	cmdValidate := exec.Command(executable, argsValidate...)
	cmdValidate.Stdout = os.Stdout
	cmdValidate.Stderr = os.Stderr
	cmdValidate.Env = utils.LoadEnvFile()

	if err := cmdValidate.Run(); err != nil {
		log.Warning("validate failed : " + err.Error())
		// exit if the config is invalid and no other process is running
		if len(processes) == 0 {
			os.Exit(1)
		}
		return
	}

	// launch the actual haproxy including the previous pids to terminate
	args := os.Args[1:]
	if len(processes) > 0 {
		args = append(args, []string{"-x", utils.LookupHAProxySocketPath(), "-sf"}...)
		args = append(args, pids()...)
	}

	log.Notice("validate successful : starting process...")

	cmd := exec.Command(executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = utils.LoadEnvFile()

	if err := cmd.AsyncRun(); err != nil {
		log.Warning("process starting failed : " + err.Error())
	}
	go func(cmd *exec.Cmd) {
		<-cmd.Terminated
		log.Notice(fmt.Sprintf("process %d terminated : %s", cmd.Process.Pid, cmd.Status()))

		// exit if termination signal was received and the last process terminated abnormally
		if terminated && cmd.ProcessState.ExitCode() != 0 {
			os.Exit(cmd.ProcessState.ExitCode())
		}

		// remove the process from tracking
		l.Lock()
		for i := range processes {
			if processes[i].cmd.Process.Pid == cmd.Process.Pid {
				processes = append(processes[:i], processes[i+1:]...)
				break
			}
		}
		l.Unlock()

		// exit if there are no more processes running
		if len(processes) == 0 {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 0 {
				os.Exit(cmd.ProcessState.ExitCode())
			} else {
				os.Exit(0)
			}
		}
	}(cmd)

	log.Notice(fmt.Sprintf("process started : pid %d status %s", cmd.Process.Pid, cmd.Status()))

	l.Lock()
	defer l.Unlock()
	// set older processes to terminating
	for i := range processes {
		if processes[i].terminateTime == nil {
			t := time.Now()
			processes[i].terminateTime = &t
		}
	}
	// add the new process
	processes = append(processes, Process{cmd: cmd})
}

func cleanupInstances() {
	if shutdownWait == nil {
		return
	}
	l.Lock()
	defer l.Unlock()
	// kill overstayed processes
	tmp := processes[:0]
	for i := range processes {
		if processes[i].terminateTime != nil && processes[i].terminateTime.Before(time.Now().Add(-*shutdownWait)) {
			proc, err := os.FindProcess(processes[i].cmd.Process.Pid)
			if err != nil {
				continue
			}
			err = proc.Signal(syscall.SIGTERM)
			// if there is an error the process does not exist and can be removed
			if err == nil {
				log.Notice(fmt.Sprintf("process killed : pid %d", processes[i].cmd.Process.Pid))
			}
		} else {
			tmp = append(tmp, processes[i])
		}
	}
	processes = tmp
}

// pids returns the PID list
func pids() []string {
	out := make([]string, 0, len(processes))
	for _, c := range processes {
		out = append(out, strconv.Itoa(c.cmd.Process.Pid))
	}
	return out
}
