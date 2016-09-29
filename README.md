# simple WAVE Monitoring and Access Protocol

sMAP driver - BOSSWAVE bridge

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
    - transform sMAP URI /a/b/c -> /a/b/s.smap/[source name]/i.something/signal/c


## Mapping sMAP to BOSSWAVE


Entity:
- we can "register" entities with the sWAP proxy.
- In the URL the sMAP driver uses to post, it contains the public key of that hash
  so that the proxy knows which key to use
PO number:
- will need to decide on a PO num that contains UUID and readings
- metadata will simply be transformed into sMAP metadata

