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
	if err := a.Cmd.Start(); err != nil {
		a.Error = err
		return a.Error
	}
	if a.Cmd.Process == nil || a.Cmd.Process.Pid < 1 {
		a.Error = errors.New("unable to create process")
		return a.Error
	}

	go func() {
		a.Error = a.Cmd.Wait()
		a.Terminated <- true
	}()

	return nil
}

func (a *Cmd) Status() string {
	if a.Cmd.Process == nil {
		return "not started"
	}

	if a.Cmd.ProcessState != nil {
		return a.Cmd.ProcessState.String()
	}

	if a.Error != nil {
		return a.Error.Error()
	}

	return "running"
}
