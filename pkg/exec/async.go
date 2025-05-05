package exec

import (
	"errors"
	"os/exec"
)

type Cmd struct {
	*exec.Cmd
	Error      error
	Terminated chan bool
}

func Command(name string, arg ...string) *Cmd {
	return &Cmd{Cmd: exec.Command(name, arg...), Error: nil, Terminated: make(chan bool)}
}

func (a *Cmd) AsyncRun() error {
	if err := a.Start(); err != nil {
		a.Error = err
		return a.Error
	}
	if a.Process == nil || a.Process.Pid < 1 {
		a.Error = errors.New("unable to create process")
		return a.Error
	}

	go func() {
		a.Error = a.Wait()
		a.Terminated <- true
	}()

	return nil
}

func (a *Cmd) Status() string {
	if a.Process == nil {
		return "not started"
	}

	if a.ProcessState != nil {
		return a.ProcessState.String()
	}

	if a.Error != nil {
		return a.Error.Error()
	}

	return "running"
}
