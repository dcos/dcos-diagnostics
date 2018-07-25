# dcos-diagnostics [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![Jenkins](https://jenkins.mesosphere.com/service/jenkins/buildStatus/icon?job=public-dcos-cluster-ops/dcos-diagnostics/dcos-diagnostics-master)](https://jenkins.mesosphere.com/service/jenkins/job/public-dcos-cluster-ops/job/dcos-diagnostics/job/dcos-diagnostics-master/) [![Go Report Card](https://goreportcard.com/badge/github.com/dcos/dcos-diagnostics)](https://goreportcard.com/report/github.com/dcos/dcos-diagnostics)
## DC/OS Distributed Diagnostics Tool & Aggregation Service
dcos-diagnostics is a monitoring agent which exposes a HTTP API for querying from the /system/health/v1 DC/OS api. dcos-diagnostics puller collects the data from agents and represents individual node health for things like system resources as well as DC/OS-specific services.

## Health Status

|Enum|Meaning    |
|----|-----------|
|  0 | working   |
|  1 | error     |
|  3 | unknown   |


## Build

```
go get github.com/dcos/dcos-diagnostics
cd $GOPATH/src/github.com/dcos/dcos-diagnostics
make install
./dcos-diagnostics --version
```

## Run
Run dcos-diagnostics once, on a DC/OS host to check systemd units:

```
dcos-diagnostics --diag
```

Get verbose log output:

```
dcos-diagnostics --diag --verbose
```

Run the dcos-diagnostics aggregation service to query all cluster hosts for health state:

```
dcos-diagnostics daemon --pull
```

Start the dcos-diagnostics health API endpoint:

```
dcos-diagnostics daemon
```

### dcos-diagnostics daemon options

<pre>
--agent-port int
    Use TCP port to connect to agents. (default 1050)

--ca-cert string
    Use certificate authority.

--command-exec-timeout int
    Set command executing timeout (default 120)

--diag
    Get diagnostics output once on the CLI. Does not expose API.

--diagnostics-bundle-dir string
    Set a path to store diagnostic bundles (default "/var/run/dcos/dcos-diagnostics/diagnostic_bundles")

--diagnostics-job-timeout int
    Set a global diagnostics job timeout (default 720)

--diagnostics-units-since string
    Collect systemd units logs since (default "24 hours ago")

--diagnostics-url-timeout int
    Set a local timeout for every single GET request to a log endpoint (default 2)

--endpoint-config string
    Use endpoints_config.json (default "/opt/mesosphere/endpoints_config.json")

--exhibitor-ip string
    Use Exhibitor IP address to discover master nodes. (default "http://127.0.0.1:8181/exhibitor/v1/cluster/status")

--force-tls
    Use HTTPS to do all requests.

--health-update-interval int
    Set update health interval in seconds. (default 60)

--master-port int
    Use TCP port to connect to masters. (default 1050)

--port int
    Web server TCP port. (default 1050)

--pull
    Try to pull checks from DC/OS hosts.

--pull-interval int
    Set pull interval in seconds. (default 60)

--pull-timeout int
    Set pull timeout. (default 3)

--verbose
    Use verbose debug output.

--version
    Print version.
</pre>


## Test
```
make test
```

Or from any submodule:

```
go test
```

