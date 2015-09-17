package fs

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/hanwen/go-fuse/fuse"
	"github.com/hanwen/go-fuse/fuse/nodefs"
	"github.com/hanwen/go-fuse/fuse/pathfs"
)

type FS struct {
	pathfs.FileSystem
	kvStore store.Store
	root    string
}

type Options struct {
	Store  string
	Addrs  []string
	Root   string
	Config store.Config
}

func NewKVFS(opts Options) (*pathfs.PathNodeFs, error) {
	kv, err := libkv.NewStore(store.Backend(opts.Store), opts.Addrs, &opts.Config)
	if err != nil {
		return nil, err
	}

	root := opts.Root
	if root != "" && root[0] == '/' {
		root = root[1:]
	}

	if !strings.HasSuffix(root, "/") {
		root = root + "/"
	}

	_, err = kv.List(root)
	if err != nil {
		return nil, fmt.Errorf("error setting root node %q: %v", opts.Root, err)
	}

	fs := pathfs.NewPathNodeFs(&FS{
		FileSystem: pathfs.NewDefaultFileSystem(),
		kvStore:    kv,
		root:       root,
	}, nil)
	return fs, nil
}

func (fs *FS) Create(name string, _ uint32, _ uint32, _ *fuse.Context) (nodefs.File, fuse.Status) {
	logrus.Debugf("Create: %v", name)
	if err := fs.kvStore.Put(name, []byte{}, nil); err != nil {
		logrus.Error(err)
		return nil, fuse.ENOENT
	}
	kv, err := fs.kvStore.Get(name)
	if err != nil {
		logrus.Error(err)
		return nil, fuse.EIO
	}
	return newFile(fs.kvStore, kv), fuse.OK
}

func (fs *FS) Open(name string, flags uint32, context *fuse.Context) (nodefs.File, fuse.Status) {
	logrus.Debugf("Open: %v", name)
	kv, err := fs.kvStore.Get(name)
	if err != nil {
		logrus.Error(err)
		return nil, fuse.ENOENT
	}
	return newFile(fs.kvStore, kv), fuse.OK
}

func (fs *FS) OpenDir(name string, context *fuse.Context) ([]fuse.DirEntry, fuse.Status) {
	name = path.Join(fs.root, name)
	logrus.Debugf("OpenDir: %v", name)
	if name == "" {
		name = "/"
	}
	kvs, err := fs.kvStore.List(name)
	if err != nil {
		logrus.Error(err)
		return nil, fuse.ENOENT
	}

	for _, kv := range kvs {
		logrus.Debug(kv.Key)
	}

	var entries []fuse.DirEntry
	for _, kv := range kvs {
		eName := kv.Key

		dir, fi := path.Split(eName)
		if dir == name+"/" && fi == "" {
			logrus.Debugf("skipping base %s", eName)
			continue
		}

		if dir != "" && dir != name+"/" && fi != "" {
			logrus.Debugf("skipping subtree %s %s", dir, eName)
			continue
		}

		var mode uint32 = fuse.S_IFREG
		if strings.HasSuffix(eName, "/") {
			eName = eName[:len(eName)-1]
			mode = fuse.S_IFDIR
			logrus.Debugf("%s dir", eName)
		} else {
			eName = fi
			logrus.Debugf("%s file", eName)
		}

		entry := fuse.DirEntry{Name: eName, Mode: mode}
		entries = append(entries, entry)
	}
	return entries, fuse.OK
}

func (fs *FS) StatFs(name string) *fuse.StatfsOut {
	logrus.Debugf("StatFs: %s", name)
	kvs, err := fs.kvStore.List(name)
	if err != nil {
		logrus.Error(err)
		return nil
	}
	return &fuse.StatfsOut{Files: uint64(len(kvs))}
}

func (fs *FS) GetAttr(name string, context *fuse.Context) (*fuse.Attr, fuse.Status) {
	logrus.Debugf("GetAttr: %s", name)
	now := time.Now()
	attr := &fuse.Attr{
		Mtime:     uint64(now.Unix()),
		Mtimensec: uint32(now.UnixNano()),
		Atime:     uint64(now.Unix()),
		Atimensec: uint32(now.UnixNano()),
		Ctime:     uint64(now.Unix()),
		Ctimensec: uint32(now.UnixNano()),
		Mode:      fuse.S_IFDIR | 0755, // default to dir
	}

	if name == "" {
		return attr, fuse.OK
	}

	kv, err := fs.kvStore.Get(name)
	if err != nil {
		if err == store.ErrKeyNotFound {
			// check if this is a dir, ie the key name might have a trailing "/"
			kv, err = fs.kvStore.Get(name + "/")
		}
		if err != nil {
			return nil, fuse.ENOENT
		}
	}

	if !strings.HasSuffix(kv.Key, "/") {
		attr.Mode = fuse.S_IFREG | 0644
		attr.Size = uint64(len(kv.Value))
	}
	return attr, fuse.OK
}

func (fs *FS) Mkdir(name string, mode uint32, context *fuse.Context) fuse.Status {
	logrus.Debugf("Mkdir: %s", name)
	if err := fs.kvStore.Put(name+"/", nil, nil); err != nil {
		logrus.Error(err)
		return fuse.ENOENT
	}
	return fuse.OK
}

func (fs *FS) Rename(oldName string, newName string, context *fuse.Context) fuse.Status {
	logrus.Debugf("Rename: %s -> %s", oldName, newName)
	kv, err := fs.kvStore.Get(oldName)
	if err != nil {
		logrus.Error(err)
		return fuse.EIO
	}

	var undo func()
	newKv, err := fs.kvStore.Get(newName)
	if err == nil {
		undo = func() {
			fs.kvStore.Put(oldName, kv.Value, nil)
			fs.kvStore.Put(newKv.Key, newKv.Value, nil)
		}
	} else {
		undo = func() {
			fs.kvStore.Put(oldName, kv.Value, nil)
			fs.kvStore.Delete(newName)
		}
	}

	if err := fs.kvStore.Put(newName, kv.Value, nil); err != nil {
		logrus.Error(err)
		return fuse.EIO
	}

	if err := fs.kvStore.Delete(oldName); err != nil {
		undo()
		logrus.Error(err)
		return fuse.EIO
	}

	return fuse.OK
}

func (fs *FS) Rmdir(name string, context *fuse.Context) fuse.Status {
	logrus.Debugf("Rmdir: %s", name)
	if err := fs.kvStore.DeleteTree(name); err != nil {
		logrus.Error(err)
		return fuse.EIO
	}
	return fuse.OK
}

func (fs *FS) Unlink(name string, context *fuse.Context) fuse.Status {
	logrus.Debugf("Unlink: %s", name)

	if err := fs.kvStore.Delete(name); err != nil {
		logrus.Error(err)
		return fuse.EIO
	}
	return fuse.OK
}

func (fs *FS) Truncate(name string, size uint64, context *fuse.Context) fuse.Status {
	kv, err := fs.kvStore.Get(name)
	if err != nil {
		return fuse.EIO
	}

	if size > uint64(len(kv.Value)) {
		size = uint64(len(kv.Value))
	}

	if err := fs.kvStore.Put(name, kv.Value[:size], nil); err != nil {
		return fuse.EIO
	}
	return fuse.OK
}

func (fs *FS) String() string {
	return "kvfs"
}
