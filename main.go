package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/cpuguy83/kvfs/fs"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/boltdb"
	"github.com/docker/libkv/store/consul"
	"github.com/docker/libkv/store/etcd"
	"github.com/docker/libkv/store/zookeeper"
)

func init() {
	consul.Register()
	boltdb.Register()
	etcd.Register()
	zookeeper.Register()
}

type stringsFlag []string

func (s *stringsFlag) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringsFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func (s *stringsFlag) GetAll() []string {
	var out []string
	for _, i := range *s {
		out = append(out, i)
	}
	return out
}

var (
	flAddrs      stringsFlag
	flMountpoint = flag.String("to", "", "Set the mount point to use")
	flStore      = flag.String("store", "", "Set the KV store type to use")
	flDebug      = flag.Bool("debug", false, "enable debug logging")
	flRoot       = flag.String("root", "", "set the root node for the store")
)

func main() {
	flag.Var(&flAddrs, "addr", "List of address to KV store")
	flag.Parse()

	if len(flAddrs) == 0 {
		logrus.Fatal("need at least one addr to connect to kv store")
	}

	if *flMountpoint == "" {
		logrus.Fatal("invalid mount point, must set the `-to` flag")
	}

	if _, err := os.Stat(*flMountpoint); err != nil {
		logrus.Fatalf("error with specified mountpoint %s: %v", *flMountpoint, err)
	}

	if *flStore == "" {
		logrus.Fatal("must specify a valid KV store")
	}

	if *flDebug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	fs, err := fs.NewKVFS(fs.Options{*flStore, flAddrs.GetAll(), *flRoot, store.Config{}})
	if err != nil {
		logrus.Fatal(err)
	}

	srv, err := fs.NewServer(*flMountpoint)
	if err != nil {
		logrus.Fatal(err)
	}

	signal.Trap(func() {
		srv.Unmount()
	})
	srv.Serve()
}
