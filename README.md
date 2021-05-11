# pgcluster

High available [PostgreSQL](https://www.postgresql.org/) cluster.


## Overview

Cluster components - PostgreSQL and monitoring agent - are delivered in a single Docker image.

The monitoring agent uses distributed key-value store for leader election and tracking cluster
state.

The following key-value stores may be used with the cluster:

* [etcd](https://etcd.io)
* [Stoa](https://github.com/vontikov/stoa)

The image exposes two ports:

* 5432 - PostgreSQL
* 3501 - monitoring and management


## Build

```
# Staged build
make image-staged
```

```
# Local build (requires [Go](https://golang.org/) to be installed)
make image
```

## Running examples

With [etcd](https://etcd.io):

```
docker-compose -f examples/docker-compose-etcd.yaml up
```

With [Stoa](https://github.com/vontikov/stoa):

```
docker-compose -f examples/docker-compose-stoa.yaml up
```
