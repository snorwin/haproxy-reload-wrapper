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
    // fetch the absolute path of the haproxy executable
    executable, err := utils.LookupExecutablePathAbs("haproxy")
    if err != nil {
        log.Emergency(err.Error())
        os.Exit(1)
    }

    // execute haproxy with the flags provided as a child process asynchronously
    cmd := exec.Command(executable, os.Args[1:]...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = utils.LoadEnvFile()
    if err := cmd.AsyncRun(); err != nil {
        log.Emergency(err.Error())
        os.Exit(1)
    }
    log.Notice(fmt.Sprintf("process %d started", cmd.Process.Pid))

    watchPaths := utils.LookupWatchPaths()

    // create a fsnotify.Watcher for config changes
    fswatch, err := fsnotify.NewWatcher()
    if err != nil {
        log.Notice(fmt.Sprintf("fsnotify watcher create failed : %v", err))
        os.Exit(1)
    }
    defer fswatch.Close()

    for _, path := range watchPaths {
        if err := fswatch.Add(path); err != nil {
            log.Notice(fmt.Sprintf("watch failed : %v", err))
            os.Exit(1)
        }
        log.Notice(fmt.Sprintf("watch : %s", path))
    }

    // flag used for termination handling
    var terminated bool
    // initialize a signal handler for SIGINT, SIGTERM and SIGUSR1 (for OpenShift)
    sigs := make(chan os.Signal, 1)
    signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

    // endless for loop which handles signals, file system events as well as termination of the child process
    for {
        select {
        case event := <-fswatch.Events:
            // only care about events which may modify the contents of the directory
            if !(event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Create)) {
                continue
            }
            log.Notice(fmt.Sprintf("fs event for %s : %v", event.Name, event.Op))

            // re-add watch if path was removed - config maps are updated by removing/adding a symlink
            if event.Has(fsnotify.Remove) {
                if err := fswatch.Add(event.Name); err != nil {
                    log.Alert(fmt.Sprintf("watch failed : %v", err))
                } else {
                    log.Notice(fmt.Sprintf("watch : %s", event.Name))
                }
            }

            // create a new haproxy process which will replace the old one after it was successfully started
            tmp := exec.Command(executable, append([]string{"-x", utils.LookupHAProxySocketPath(), "-sf", strconv.Itoa(cmd.Process.Pid)}, os.Args[1:]...)...)
            tmp.Stdout = os.Stdout
            tmp.Stderr = os.Stderr
            tmp.Env = utils.LoadEnvFile()
            if err := tmp.AsyncRun(); err != nil {
                log.Warning(err.Error())
                log.Warning("reload failed")
                continue
            }
            log.Notice(fmt.Sprintf("process %d started", tmp.Process.Pid))
            select {
            case <-cmd.Terminated:
                cmd = tmp
            }
        case sig := <-sigs:
            log.Notice(fmt.Sprintf("signal %s received", sig))
            terminated = true
        }
        if terminated {
            break
        }
    }
}