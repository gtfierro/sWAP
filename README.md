# sMAP to WAVE Monitoring and Access Protocol

sMAP driver - BOSSWAVE bridge

## The Problem

There exist a whole family of sMAP drivers in the [master](https://github.com/SoftwareDefinedBuildings/smap/tree/master/python/smap/drivers)
and [unitoftime](https://github.com/SoftwareDefinedBuildings/smap/tree/unitoftime/python/smap/drivers) branches of sMAP. In moving forward
deployments to [BOSSWAVE](https://github.com/immesys/bw2), we need a way of "porting" these sMAP drivers to communicate with the BOSSWAVE
network with minimal intervention.

## The Solution

sWAP is a local server process that acts as an endpoint for running sMAP sources. In the configuration file, the sWAP proxy is listed as
though it were another archiver, but with a specially constructed URL that gives sWAP the necessary information to act as a proxy
into BOSSWAVE. It persists any received metadata in BOSSWAVE, and publishes all received timeseries data *without any changes made to the sMAP
driver code*.

A single sWAP server can act as the proxy for many running sMAP sources.

### Installing and Running sWAP

**Note**: you do need a local [BOSSWAVE](https://github.com/immesys/bw2) router running on your system. If you run `bw2 status` in a terminal,
you should get back something like

```
 ╔╡127.0.0.1:28589 2.4.15 'Hadron'
 ╚╡peers=19 block=1143398 age=16s
BW2 Local Router status:
    Peer count: 19
 Current block: 1143398
    Seen block: 1143398
   Current age: 16s
    Difficulty: 560251374
```

If you don't, then try `curl get.bw2.io/agent | sh` to install BOSSWAVE.

---

To install, download a [binary](https://github.com/gtfierro/sWAP/releases/tag/v0.2) or run

```bash
go get -u github.com/gtfierro/sWAP
go install github.com/gtfierro/sWAP
```

To run, we invoke the `server` subcommand of the sWAP binary, which has two configurable options:
* `address`: the address on which the sWAP HTTP server listens (defaults to `localhost:8078`)
* `pidfile`: the location of the server's PID file (this is important!)

The default options are usually fine, but it is important to make sure that the server is only listening on local interfaces, otherwise
any entity can publish data using your entity; this is an equivalent security model to the existing local BW agent.

Here's the invocation of the server, with the default options specified explicitly:

```bash
sWAP server -a localhost:8078 -pf sWAP.pid
```

You should see output like:

```
1475286578312426551 [Info] Connected to BOSSWAVE router version 2.4.15 'Hadron'
NOTICE Sep 30 18:49:38 server.go:46 ▶ Serving on localhost:8078...
WARNING Sep 30 18:49:38 store.go:56 ▶ Waiting for signal
```

You are now ready to register entities

### Registering Entities

Every resource on BOSSWAVE is associated with an "entity", which is a public/private key pair. As opposed to typical public/private key models
in which a keypair is associated with a user, keypairs in BOSSWAVE are associated with an instance of a service or driver that interacts with
BOSSWAVE.

For each sMAP source, we need to provide:
* an entity that constitutes the "identity" of that driver
* a base URI to publish on; the entity of the driver will need permission to publish on that URI


#### Entity

For detailed instructions on creating an entity, look at [https://github.com/immesys/bw2#getting-started](https://github.com/immesys/bw2#getting-started).
Here, we'll only provide the "quick'n'dirty" method that has the lines to run, but not much of the explanation.

First, we create an entity to use for our sMAP driver:

```
bw2 mke -c "Oski Bear <oski@bear.com>" -m "Oski's example sMAP driver" -e 20y -o examplesmap.ent
```

Next, we register that entity with our running sWAP server; this requires knowing the location of the server's PID file (by default, it is `sWAP.pid`
in the same directory as the running server)

```
sWAP register examplesmap.ent -pf sWAP.pid
```

This will pause the sWAP server momentarily to allow it to parse the entity file, pull out the VK, and create a BOSSWAVE client instance
that it will use to forward messages on BOSSWAVE.

You will need the VK of the entity to form the URI for the driver. To extract this, simply run

```
bw2 i examplesmap.ent
```

and find the VK field which should look something like

```
Entity VK: HSdoIH4-A_bO58U-cmQa-dmtx7Ub6k2f8BiStrW1mg4=
```

#### Base URI

We need to decide on a URI that the sMAP driver will publish on.

For a given base URI (the prefix of the real URI) of `scratch.ns/smap/exampledriver1`, we may have a sMAP driver that publishes
timeseries data on `/sensors/sensor1` and `/sensors/sensor2`, and metadata on `/`,`/sensors`,`/sensors/sensor1` and `/sensors/sensor2`.

sWAP will transform this into the following:

Metadata on:
* `scratch.ns/smap/exampledriver1/!meta/{key}`
* `scratch.ns/smap/exampledriver1/sensors/!meta/{key}`
* `scratch.ns/smap/exampledriver1/sensors/sensor1/!meta/{key}`
* `scratch.ns/smap/exampledriver1/sensors/sensor2/!meta/{key}`

Timeseries on
* `scratch.ns/smap/exampledriver1/sensors/sensor1`
* `scratch.ns/smap/exampledriver1/sensors/sensor2`

You will need Publish and Consume permissions on that base URI prefix (`PC*` permissions on `<base uri>/*`)

### Publishing on BOSSWAVE

Now, we can configure a sMAP source to publish to the sWAP server. In the configuration file for the sMAP source, we configure
the sWAP server as a `ReportDeliveryLocation` as we would a normal sMAP archiver. The difference is in how we construct the URL.

The `ReportDeliveryLocation` should follow this template:

```ini
# choose whatever report number you want
[report 1]
ReportDeliveryLocation = http://localhost:8078/add/<VK of your entity>/uri/<base URI>
```

An example:

```ini
[report 0]
ReportDeliveryLocation = http://localhost:8078/add/HSdoIH4-A_bO58U-cmQa-dmtx7Ub6k2f8BiStrW1mg4=/uri/scratch.ns/smap/exampledriver1
```

Now, start the sMAP driver as you usually would, and observe the messages being published on BOSSWAVE!

All metadata will be exposed as BOSSWAVE metadata

All timeseries will be published as PO 2.0.9.1

```go
type TimeseriesReading struct {
	UUID  string
	Time  int64
	Value float64
}
```

---

## Protocol Comparison

### sMAP

sMAP drivers publish sMAP messages to an HTTP destination, which can be at any URI

Each message contains:
- UUID: unique identifier for the stream
- Readings: a list of time-value pairs
- Metadata/Properties: key-value pairs
- Path: where in the sMAP hierarchy a message is

### BOSSWAVE

To publish on BOSSWAVE, we need:
- an entity: this is a public/private key pair that identifies us to the mesh:
    - we will need a way to determine the entity for a sMAP driver
- PO number: determines the format of the message:
    - this will likely be the same for all sMAP messages
- URI: this is where BOSSWAVE messages are published:
    - options?
    - prefix each of the sMAP paths with some base URI:
        - benefit: simple, extensible
        - cons: don't get signal/slot URI structure
    - transform sMAP URI /a/b/c -> /a/b/s.smap/[source name]/i.something/signal/c:
        - what is "something"? driver file name?
        - this can be indicated in the URI the sMAP driver uses to report
        - cons: this isn't obvious to do: we don't know in advance what the timeseries URIs are,
          and because sMAP timeseries can be nested in each other (a uri can have sub-uris and a timeseries),
          this transformation isn't straightforward if we want to preserve where metadata is persisted on URIs


## Mapping sMAP to BOSSWAVE

Entity:
- we can "register" entities with the sWAP proxy.
- In the URL the sMAP driver uses to post, it contains the public key of that hash
  so that the proxy knows which key to use
PO number:
- will need to decide on a PO num that contains UUID and readings (2.0.9.1)
- metadata will simply be transformed from sMAP metadata into BOSSWAVE metadata persisted on URIs
URI:
- there will be some transformation on the URI the sMAP driver sends
- transform sMAP URI /a/b/c -> /a/b/s.smap/[source name]/i.something/signal/c:
    - what is the interface? "something"? driver file name?
    - this can be indicated in the URI the sMAP driver uses to report
Reliability:
- the proxy checks its connectivity to BOSSWAVE and buffers messages if it knows they cannot be sent
