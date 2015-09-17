package fs

import (
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libkv/store"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
)

type file struct {
	kvStore store.Store
	kv      *store.KVPair
	nodefs.File
}

func newFile(s store.Store, kv *store.KVPair) nodefs.File {
	return &file{s, kv, nodefs.NewDefaultFile()}
}

func (f *file) String() string {
	return f.kv.Key
}

func (f *file) Read(buf []byte, offset int64) (fuse.ReadResult, fuse.Status) {
	logrus.Debugf("Read: %s", string(f.kv.Value))

	end := int(offset) + len(buf)
	if end > len(f.kv.Value) {
		end = len(f.kv.Value)
	}

	copy(buf, f.kv.Value[offset:end])
	return fuse.ReadResultData(buf), fuse.OK
}

func (f *file) Write(data []byte, off int64) (uint32, fuse.Status) {
	val := f.kv.Value[:off]
	val = append(val, data...)
	copy(val[off:], data)

	if err := f.kvStore.Put(f.kv.Key, val, nil); err != nil {
		logrus.Error(err)
		return uint32(0), fuse.EIO
	}
	return uint32(len(data)), fuse.OK
}

func (f *file) GetAttr(out *fuse.Attr) fuse.Status {
	logrus.Debugf("FGetAttr %s", f.kv.Key)
	now := time.Now()
	out.Mtime = uint64(now.Unix())
	out.Mtimensec = uint32(now.UnixNano())
	out.Atime = uint64(now.Unix())
	out.Atimensec = uint32(now.UnixNano())
	out.Ctime = uint64(now.Unix())
	out.Ctimensec = uint32(now.UnixNano())

	if f.kv == nil || strings.HasSuffix(f.kv.Key, "/") || f.kv.Key == "" {
		out.Mode = fuse.S_IFDIR | 0755
		return fuse.OK
	}

	if len(f.kv.Value) > 0 {
		out.Mode = fuse.S_IFREG | 0644
		out.Size = uint64(len(f.kv.Value))
	}
	return fuse.OK
}
