# HAProxy Reload Wrapper

![Build](https://img.shields.io/github/workflow/status/snorwin/haproxy-reload-wrapper/Publish%20(main)?label=Build%20%28main%29&style=flat-square)
![Release](https://img.shields.io/github/workflow/status/snorwin/haproxy-reload-wrapper/Publish%20(Release)?label=Build%20%28Release%29&style=flat-square)
![E2E Tests](https://img.shields.io/github/workflow/status/snorwin/haproxy-reload-wrapper/E2E%20Tests?label=E2E%20Tests&style=flat-square)
[![Go Report Card](https://goreportcard.com/badge/github.com/snorwin/haproxy-reload-wrapper?style=flat-square)](https://goreportcard.com/report/github.com/snorwin/haproxy-reload-wrapper)
[![Releases](https://img.shields.io/github/v/release/snorwin/haproxy-reload-wrapper?style=flat-square&label=Release)](https://github.com/snorwin/haproxy-reload-wrapper/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](https://opensource.org/licenses/MIT)

The haproxy-reload-wrapper watches the HAProxy configuration file using an inotify watcher and, if a change is detected, performs a hitless reload by transferring listening sockets from the old HAProxy process to the new, reloaded process. If the new HAProxy process fails to start or the changed configuration is invalid, the old process continues to operate to avoid any interruptions. More details about the reload mechanism in HAProxy can be found in the following blog post: [Truly Seamless Reloads with HAProxy – No More Hacks!](https://www.haproxy.com/blog/truly-seamless-reloads-with-haproxy-no-more-hacks/).

## Features
- Watch for changes in the configuration file and trigger seamless relaods of HAProxy
- Graceful signal (termination) handling and transparent management of HAProxy processes
- Support for configuration files from mounted `ConfigMap`

## How to use
1. Configure a socket with the `expose-fd listeners` option in the `haproxy.cfg` file:
  ```
  global
    stats socket /var/run/haproxy.sock mode 600 level admin expose-fd listeners
  ```
2. Set the `HAPROXY_SOCKET` environment variable to the path of the socket if it is different from the default path: `/var/run/haproxy.sock`.
3. Replace the `docker.io/haproxy` image with the `ghcr.io/snorwin/haproxy` image on container platforms or compile the source code and run `./haproxy-reload-wrapper` on a Linux system. As an example, check out the [Helm chart](test/helm) used for the tests.
4. Modify the configuration file and let the magic happen.✨
