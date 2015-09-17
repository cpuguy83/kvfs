# kvfs
FUSE based filesystem for KV stores  
Connects to a supported KV store and maps the structure to a FUSE based filesystem.

Supports:
 - etcd
 - zookeeper
 - consul
 - boltdb

### Usage

```bash
$ sudo ./kvfs -store consul -addr 1.2.3.4:8500 -addr 1.2.3.5:8500 -to /data
```
This command initiates a connection to the consul servers at the specified addresses and mounts the structure to `/data` on your filesystem.


### Build

```bash
$ godep get
$ godep go build
```

## THANKS
https://github.com/docker/libkv

https://github.com/hanwen/go-fuse
