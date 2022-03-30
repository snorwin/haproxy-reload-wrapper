package utils

import (
	"os"
	"os/exec"
	"path/filepath"
)

// LookupExecutablePathAbs lookup the $PATH to find the absolut path of an executable
func LookupExecutablePathAbs(executable string) (string, error) {
	file, err := exec.LookPath(executable)
	if err != nil {
		return "", err
	}

	return filepath.Abs(file)
}

// LookupHAProxyConfigFile lookup the program arguments to find the config file path (default: "/etc/haproxy/haproxy.cfg")
func LookupHAProxyConfigFile() string {
	file := "/etc/haproxy/haproxy.cfg"
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-f" && i+1 < len(os.Args) {
			file = os.Args[i+1]
		}
	}

	return file
}

// LookupHAProxySocketPath lookup the value of HAPROXY_SOCKET environment variable (default:"/var/run/haproxy.sock")
func LookupHAProxySocketPath() string {
	if path, ok := os.LookupEnv("HAPROXY_SOCKET"); ok {
		return path
	}

	return "/var/run/haproxy.sock"
}
