# Basic Example

This basic example runs a python web server service in a Ubuntu VM.

## Setup

Perform setup described [here](../README.md).

## Run

In one terminal, start Nomad:

``` shell-session
nomad agent -dev -config ./config.hcl
```

In another terminal, run the python-server job:

``` shell-session
nomad run ./python-server.nomad.hcl
```

After the service is healthy, fetch the dynamic port. First get the allocation ID for the python-server job:

``` shell-session
ALLOC_ID="$(nomad job status -json python-server | jq -r '.[].Allocations[0].ID')"
```

Next, get the dynamic port from the allocation using the allocation ID:

``` shell-session
PORT="$(nomad alloc status -json $ALLOC_ID | jq -r '.Resources.Networks[0].DynamicPorts[0].Value')"
```

Finally, use the port to make a request to the service:

``` shell-session
curl 127.0.0.1:$PORT
```
