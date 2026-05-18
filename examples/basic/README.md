# Basic Example

This basic example runs a python web server service in a Ubuntu VM.

## Setup

Perform setup described [here](../README.md).

## Run

In one terminal, start Nomad:

``` shellsession
nomad agent -dev -config ./config.hcl
```

In another terminal, run the python-server job:

``` shellsession
nomad run ./python-server.nomad.hcl
```

After the service is healthy, fetch the dynamic port. First get the allocation ID for the python-server job:

``` shellsession
$ ALLOC_ID="$(nomad job allocs -json python-server | jq -r '.[].ID')"
```

Next, get the dynamic port from the allocation using the allocation ID:

``` shellsession
$ PORT="$(nomad alloc status -json $ALLOC_ID | jq -r '.Resources.Networks[0].DynamicPorts[0].Value')"
```

Finally, use the port to make a request to the service:

``` shellsession
$ curl 127.0.0.1:$PORT
```
